package gitlab

import (
	"fmt"
	"net/url"
	"strings"

	"repository-migrator/internal/logs"

	"github.com/go-resty/resty/v2"
)

type Client struct {
	baseURL string
	token   string
	http    *resty.Client
}

var (
	// ErrProjectPathTaken indicates the requested project path (slug) is already taken
	ErrProjectPathTaken = fmt.Errorf("gitlab: project path already taken")
)

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type Project struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	HttpURLToRepo string `json:"http_url_to_repo"`
	DefaultBranch string `json:"default_branch"`
}

type Group struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	FullPath string `json:"full_path"`
}

func NewClient(baseURL, token string) *Client {
	c := resty.New()
	c.SetHeader("PRIVATE-TOKEN", token)
	c.SetBaseURL(strings.TrimRight(baseURL, "/") + "/api/v4")
	c.OnAfterResponse(func(_ *resty.Client, resp *resty.Response) error {
		// Log method, URL path and status; truncate body to 2k
		body := string(resp.Body())
		if len(body) > 2000 {
			body = body[:2000] + "..."
		}
		logs.AppendRunDetailCurrent(fmt.Sprintf("gitlab %s %s -> %d %s", resp.Request.Method, resp.Request.URL, resp.StatusCode(), body))
		return nil
	})
	return &Client{baseURL: baseURL, token: token, http: c}
}

func (c *Client) CurrentUser() (User, error) {
	var u User
	resp, err := c.http.R().SetResult(&u).Get("/user")
	if err != nil {
		return u, err
	}
	if resp.IsError() {
		return u, fmt.Errorf("get current user failed: %s", resp.Status())
	}
	return u, nil
}

// CreateProjectInUserNamespace creates a project under the authenticated user's namespace.
func (c *Client) CreateProjectInUserNamespace(name string) (Project, error) {
	var p Project
	body := map[string]any{
		"name":       name,
		"path":       repoPathFromName(name),
		"visibility": "private",
	}
	resp, err := c.http.R().SetBody(body).SetResult(&p).Post("/projects")
	if err != nil {
		return p, err
	}
	if resp.IsError() {
		// If project exists, try to fetch it (under current user's namespace)
		if resp.StatusCode() == 400 && strings.Contains(string(resp.Body()), "has already been taken") {
			u, uerr := c.CurrentUser()
			if uerr == nil {
				return c.GetProjectByFullPath(u.Username + "/" + repoPathFromName(name))
			}
			return p, fmt.Errorf("create project failed and user lookup failed: %s - %s", resp.Status(), string(resp.Body()))
		}
		return p, fmt.Errorf("create project failed: %s - %s", resp.Status(), string(resp.Body()))
	}
	return p, nil
}

// CreateProjectInNamespace creates a project under a specific namespace (group/subgroup) by ID.
func (c *Client) CreateProjectInNamespace(name, path string, namespaceID int, namespaceFullPath string) (Project, error) {
	var p Project
	if path == "" {
		path = repoPathFromName(name)
	}
	body := map[string]any{
		"name":         name,
		"path":         path,
		"namespace_id": namespaceID,
		"visibility":   "private",
	}
	resp, err := c.http.R().SetBody(body).SetResult(&p).Post("/projects")
	if err != nil {
		return p, err
	}
	if resp.IsError() {
		if resp.StatusCode() == 400 && strings.Contains(string(resp.Body()), "has already been taken") {
			if strings.TrimSpace(namespaceFullPath) != "" {
				proj, gerr := c.GetProjectByFullPath(namespaceFullPath + "/" + path)
				if gerr == nil {
					return proj, nil
				}
				// If not found in intended namespace, signal that the path is taken elsewhere
				return p, ErrProjectPathTaken
			}
			return p, ErrProjectPathTaken
		}
		return p, fmt.Errorf("create project failed: %s - %s", resp.Status(), string(resp.Body()))
	}
	return p, nil
}

// GetGroupByFullPath fetches a group (or subgroup) by its full path (e.g., "org" or "org/subgroup").
func (c *Client) GetGroupByFullPath(fullPath string) (Group, error) {
	var g Group
	enc := url.PathEscape(fullPath)
	resp, err := c.http.R().SetResult(&g).Get("/groups/" + enc)
	if err != nil {
		return g, err
	}
	if resp.IsError() {
		return g, fmt.Errorf("get group failed: %s - %s", resp.Status(), string(resp.Body()))
	}
	return g, nil
}

// CreateSubgroup creates a subgroup under a parent group by parent ID
func (c *Client) CreateSubgroup(parentID int, name, path string) (Group, error) {
	var g Group
	if strings.TrimSpace(path) == "" {
		path = repoPathFromName(name)
	}
	body := map[string]any{
		"name":       name,
		"path":       path,
		"parent_id":  parentID,
		"visibility": "private",
	}
	resp, err := c.http.R().SetBody(body).SetResult(&g).Post("/groups")
	if err != nil {
		return g, err
	}
	if resp.IsError() {
		return g, fmt.Errorf("create subgroup failed: %s - %s", resp.Status(), string(resp.Body()))
	}
	return g, nil
}

func (c *Client) GetProjectByPath(path string) (Project, error) {
	var p Project
	// URL-encode the path for API call
	enc := url.PathEscape(path)
	resp, err := c.http.R().SetResult(&p).Get("/projects/" + enc)
	if err != nil {
		return p, err
	}
	if resp.IsError() {
		return p, fmt.Errorf("get project failed: %s - %s", resp.Status(), string(resp.Body()))
	}
	return p, nil
}

// GetProjectByFullPath fetches a project by its full path (e.g., "group/subgroup/project").
func (c *Client) GetProjectByFullPath(fullPath string) (Project, error) {
	return c.GetProjectByPath(fullPath)
}

// Protected branches API
type ProtectedBranch struct {
	Name string `json:"name"`
}

func (c *Client) ListProtectedBranches(projectID int) ([]ProtectedBranch, error) {
	var out []ProtectedBranch
	resp, err := c.http.R().SetResult(&out).Get(fmt.Sprintf("/projects/%d/protected_branches", projectID))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("list protected branches failed: %s - %s", resp.Status(), string(resp.Body()))
	}
	return out, nil
}

func (c *Client) UnprotectBranch(projectID int, branch string) error {
	enc := url.PathEscape(branch)
	resp, err := c.http.R().Delete(fmt.Sprintf("/projects/%d/protected_branches/%s", projectID, enc))
	if err != nil {
		return err
	}
	if resp.StatusCode() == 404 {
		return nil // not protected
	}
	if resp.IsError() {
		return fmt.Errorf("unprotect branch failed: %s - %s", resp.Status(), string(resp.Body()))
	}
	return nil
}

// ProtectBranch sets push/merge access; 40=Maintainers
func (c *Client) ProtectBranch(projectID int, branch string, pushAccess, mergeAccess int) error {
	body := map[string]any{
		"name":               branch,
		"push_access_level":  pushAccess,
		"merge_access_level": mergeAccess,
	}
	resp, err := c.http.R().SetBody(body).Post(fmt.Sprintf("/projects/%d/protected_branches", projectID))
	if err != nil {
		return err
	}
	if resp.IsError() {
		return fmt.Errorf("protect branch failed: %s - %s", resp.Status(), string(resp.Body()))
	}
	return nil
}

func repoPathFromName(name string) string {
	s := strings.ToLower(name)
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.Trim(s, "-/.")
	return s
}

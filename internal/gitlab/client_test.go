package gitlab

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"repository-migrator/internal/logs"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(w, r)
	}))
}

func TestCurrentUser(t *testing.T) {
	// capture run logs to a temp file
	runPath := t.TempDir() + "/run.log"
	logs.SetCurrentRunPath(runPath)

	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/user" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("PRIVATE-TOKEN") != "tok" {
			t.Fatalf("missing or wrong token header")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "username": "alice"})
	})
	defer server.Close()

	c := NewClient(server.URL, "tok")
	u, err := c.CurrentUser()
	if err != nil { t.Fatalf("CurrentUser: %v", err) }
	if u.Username != "alice" || u.ID != 1 { t.Fatalf("unexpected user: %#v", u) }
}

func TestCreateProjectInNamespace_PathTaken_ErrProjectPathTaken(t *testing.T) {
	runPath := t.TempDir() + "/run.log"
	logs.SetCurrentRunPath(runPath)

	calls := 0
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"message":"has already been taken"}`))
			return
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/"):
			// Simulate not found in intended namespace
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"404 Project Not Found"}`))
			return
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	defer server.Close()

	c := NewClient(server.URL, "tok")
	_, err := c.CreateProjectInNamespace("name", "path", 123, "group/sub")
	if err == nil {
		t.Fatal("expected ErrProjectPathTaken")
	}
	if err != ErrProjectPathTaken {
		t.Fatalf("got %v want ErrProjectPathTaken", err)
	}
	if calls < 2 { t.Fatalf("expected multiple calls, got %d", calls) }
}

func TestBranchProtectionEndpoints(t *testing.T) {
	runPath := t.TempDir() + "/run.log"
	logs.SetCurrentRunPath(runPath)

	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/projects/1/protected_branches/") {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/1/protected_branches" {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"name":"main"}`))
			return
		}
		t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
	})
	defer server.Close()

	c := NewClient(server.URL, "tok")
	if err := c.UnprotectBranch(1, "main"); err != nil {
		t.Fatalf("UnprotectBranch: %v", err)
	}
	if err := c.ProtectBranch(1, "main", 40, 40); err != nil {
		t.Fatalf("ProtectBranch: %v", err)
	}
}

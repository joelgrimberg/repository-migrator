package main

import (
	"strings"
	"testing"
)

func TestValidateBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid https", "https://gitlab.com", false},
		{"valid http", "http://gitlab.example.com", false},
		{"with path", "https://gitlab.com/api", false},
		{"no scheme", "gitlab.com", true},
		{"invalid url", "https://[invalid", true},
		{"empty", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBaseURL(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateBaseURL(%q) err=%v wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestValidateGitURL(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"SSH form", "git@gitlab.com:user/repo.git", false},
		{"HTTPS", "https://gitlab.com/user/repo.git", false},
		{"HTTP", "http://gitlab.com/user/repo.git", false},
		{"invalid", "not-a-url", true},
		{"empty", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGitURL(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateGitURL(%q) err=%v wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestInferRepoName(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{"SSH with .git", "git@gitlab.com:user/my-repo.git", "my-repo"},
		{"SSH without .git", "git@gitlab.com:user/my-repo", "my-repo"},
		{"HTTPS with .git", "https://gitlab.com/user/my-repo.git", "my-repo"},
		{"HTTPS without .git", "https://gitlab.com/user/my-repo", "my-repo"},
		{"HTTPS with path", "https://gitlab.com/group/subgroup/repo.git", "repo"},
		{"empty", "", ""},
    {"invalid-like string (parsed as path)", "not-a-url", "not-a-url"},
		{"root path", "https://gitlab.com/", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := inferRepoName(tc.input)
			if got != tc.expected {
				t.Fatalf("inferRepoName(%q) = %q want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestAddTokenToHTTPURL(t *testing.T) {
	url := "https://gitlab.com/user/repo.git"
	token := "secret-token"
	got, err := addTokenToHTTPURL(url, token)
	if err != nil {
		t.Fatalf("addTokenToHTTPURL error: %v", err)
	}
	if !strings.Contains(got, "oauth2") || !strings.Contains(got, token) {
		t.Fatalf("addTokenToHTTPURL(%q, %q) = %q missing oauth2 or token", url, token, got)
	}
	if !strings.HasPrefix(got, "https://oauth2:") {
		t.Fatalf("addTokenToHTTPURL(%q, %q) = %q should start with https://oauth2:", url, token, got)
	}
}

func TestSanitizeProjectPathSlug(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "my-repo", "my-repo"},
		{"uppercase", "My-Repo", "my-repo"},
		{"spaces", "my repo", "my-repo"},
		{"with .git", "my-repo.git", "my-repo"},
		{"special chars", "my@repo#123", "my-repo-123"},
		{"multiple dashes", "my---repo", "my-repo"},
		{"leading/trailing dash", "-my-repo-", "my-repo"},
		{"leading/trailing dot", ".my-repo.", "my-repo"},
		{"underscores", "my_repo", "my_repo"},
		{"mixed", "My Repo@123.git", "my-repo-123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeProjectPathSlug(tc.input)
			if got != tc.expected {
				t.Fatalf("sanitizeProjectPathSlug(%q) = %q want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestValidateProjectPathSlug(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "my-repo", false},
		{"with underscore", "my_repo", false},
		{"with dot", "my.repo", false},
		{"with numbers", "repo123", false},
		{"empty", "", true},
		{"ends with .git", "repo.git", true},
		{"ends with .atom", "repo.atom", true},
		{"starts with dash", "-repo", true},
		{"starts with underscore", "_repo", true},
		{"starts with dot", ".repo", true},
		{"ends with dash", "repo-", true},
		{"ends with underscore", "repo_", true},
		{"ends with dot", "repo.", true},
		{"invalid chars", "repo@123", true},
		{"uppercase", "MyRepo", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateProjectPathSlug(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateProjectPathSlug(%q) err=%v wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestSanitizeProjectName(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "My Repo", "My Repo"},
		{"with control chars", "My\tRepo\n", "My Repo"},
		{"multiple spaces", "My   Repo", "My Repo"},
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"leading/trailing space", "  My Repo  ", "My Repo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeProjectName(tc.input)
			if got != tc.expected {
				t.Fatalf("sanitizeProjectName(%q) = %q want %q", tc.input, got, tc.expected)
			}
		})
	}
}

// Note: addTokenToHTTPURL relies on url.Parse which accepts path-only
// strings without error; invalid-URL tests are not applicable here.


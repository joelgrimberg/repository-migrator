package main

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
)

//go:embed templates/security-pipeline.yml
var ciTemplate []byte

const embeddedTemplateSource = "embedded:templates/security-pipeline.yml"

func gitIdentityName() string {
	if v := strings.TrimSpace(os.Getenv("GIT_COMMITTER_NAME")); v != "" {
		return v
	}
	return "Repository Migrator"
}

func gitIdentityEmail() string {
	if v := strings.TrimSpace(os.Getenv("GIT_COMMITTER_EMAIL")); v != "" {
		return v
	}
	return "migrator@local"
}

// resolveCITemplate returns the bytes that should be committed onto target as
// .gitlab-ci.yml, along with a short label describing which source provided
// them. Precedence: CI_TEMPLATE_FILE env > --ci-template flag > embedded
// fallback. Missing or unreadable paths produce an error rather than silently
// falling back, so operator misconfiguration surfaces loudly.
func resolveCITemplate() (content []byte, source string, err error) {
	if v := strings.TrimSpace(os.Getenv("CI_TEMPLATE_FILE")); v != "" {
		b, rerr := os.ReadFile(v)
		if rerr != nil {
			return nil, "", fmt.Errorf("CI_TEMPLATE_FILE=%q: %w", v, rerr)
		}
		return b, v, nil
	}
	if ciTemplateFlag != nil {
		if path := strings.TrimSpace(*ciTemplateFlag); path != "" {
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				return nil, "", fmt.Errorf("--ci-template=%q: %w", path, rerr)
			}
			return b, path, nil
		}
	}
	return ciTemplate, embeddedTemplateSource, nil
}

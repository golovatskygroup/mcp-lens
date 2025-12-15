package testutil

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ParseGitHubPullRequestURL parses URLs like:
//
//	https://github.com/<owner>/<repo>/pull/<number>
//
// It returns repo as "owner/repo".
func ParseGitHubPullRequestURL(raw string) (repo string, number int, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0, fmt.Errorf("empty url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", 0, err
	}
	if !strings.EqualFold(u.Host, "github.com") {
		return "", 0, fmt.Errorf("unsupported host %q", u.Host)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 {
		return "", 0, fmt.Errorf("unexpected path %q", u.Path)
	}
	if parts[2] != "pull" {
		return "", 0, fmt.Errorf("unexpected path %q (expected /<owner>/<repo>/pull/<number>)", u.Path)
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil || n <= 0 {
		return "", 0, fmt.Errorf("invalid pull request number in %q", u.Path)
	}
	return parts[0] + "/" + parts[1], n, nil
}

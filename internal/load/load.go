package load

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/ossf/si-tooling/v2/si"
)

// LoadSecurityInsights loads SecurityInsights from a local file path or HTTP(S) URL.
// If source is a path that exists as a file, it reads with os.ReadFile and calls si.Load.
// If source looks like http:// or https://, it GETs the URL and calls si.Load.
func LoadSecurityInsights(source string) (*si.SecurityInsights, error) {
	var contents []byte
	var err error

	if isURL(source) {
		contents, err = fetchURL(source)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch URL: %w", err)
		}
	} else {
		contents, err = os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	}

	insights, err := si.Load(contents)
	if err != nil {
		return nil, fmt.Errorf("failed to load security insights: %w", err)
	}
	return insights, nil
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func fetchURL(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// LoadSecurityInsightsFromGitHub loads SecurityInsights from a public GitHub repository
// using si.Read. path defaults to si.SecurityInsightsFilename if empty.
func LoadSecurityInsightsFromGitHub(owner, repo, path string) (*si.SecurityInsights, error) {
	if path == "" {
		path = si.SecurityInsightsFilename
	}
	insights, err := si.Read(owner, repo, path)
	if err != nil {
		return nil, fmt.Errorf("failed to read from GitHub: %w", err)
	}
	return &insights, nil
}

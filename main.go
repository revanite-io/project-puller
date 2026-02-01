package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ossf/project-puller/internal/load"
	"github.com/ossf/si-tooling/v2/si"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var (
	source   string
	github   string
	dir      string
	quiet    bool
)

var rootCmd = &cobra.Command{
	Use:   "project-puller [file-or-url]",
	Short: "Clone or pull repositories listed in a security-insights file",
	Long:  "Loads a security-insights YAML (from a local file or URL), extracts project.repositories, and runs git clone or git pull for each.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  run,
}

func init() {
	rootCmd.Flags().StringVarP(&source, "source", "s", "", "Path to security-insights file or HTTP(S) URL")
	rootCmd.Flags().StringVarP(&github, "github", "g", "", "Load from GitHub as owner/repo[/path] (e.g. org/repo or org/repo/dir/security-insights.yml)")
	rootCmd.Flags().StringVarP(&dir, "output", "", "", "Target directory for cloned repositories")
	rootCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress git output")
}

func run(cmd *cobra.Command, args []string) error {
	var insights *si.SecurityInsights
	var err error

	if github != "" {
		owner, repo, path := parseGitHubFlag(github)
		if owner == "" || repo == "" {
			return fmt.Errorf("--github must be owner/repo or owner/repo/path")
		}
		insights, err = load.LoadSecurityInsightsFromGitHub(owner, repo, path)
	} else {
		src := source
		if len(args) > 0 {
			src = args[0]
		}
		if src == "" && source != "" {
			src = source
		}
		if src == "" {
			return fmt.Errorf("provide a file path, URL, or use --g owner/repo")
		}
		insights, err = load.LoadSecurityInsights(src)
	}
	if err != nil {
		return err
	}
	if insights.Project == nil || len(insights.Project.Repositories) == 0 {
		return fmt.Errorf("security insights file has no project or repositories listed")
	}

	if dir == "" && insights.Project.Name != "" {
		dir = insights.Project.Name
	}


	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory %s: %w", dir, err)
	}

	usedNames := make(map[string]bool)
	for _, r := range insights.Project.Repositories {
		repoURL := string(r.Url)
		dirName := repoDirName(r, repoURL, usedNames)
		usedNames[dirName] = true
		targetPath := filepath.Join(dir, dirName)

		if err := cloneOrPull(targetPath, repoURL); err != nil {
			return fmt.Errorf("git failed for %s: %w", dirName, err)
		}
	}
	return nil
}

func parseGitHubFlag(s string) (owner, repo, path string) {
	parts := strings.SplitN(s, "/", 3)
	if len(parts) >= 2 {
		owner = parts[0]
		repo = parts[1]
		if len(parts) == 3 {
			path = parts[2]
		}
	}
	return owner, repo, path
}

func repoDirName(r si.ProjectRepository, repoURL string, usedNames map[string]bool) string {
	name := strings.TrimSpace(r.Name)
	if name != "" {
		dirName := sanitizeDirName(name)
		if dirName != "" && !usedNames[dirName] {
			return dirName
		}
	}
	base := lastPathComponent(repoURL)
	candidate := base
	for i := 0; usedNames[candidate]; i++ {
		if i == 0 {
			candidate = base
		} else {
			candidate = fmt.Sprintf("%s-%d", base, i)
		}
	}
	return candidate
}

func sanitizeDirName(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 {
			return -1
		}
		return r
	}, s)
}

func lastPathComponent(url string) string {
	url = strings.TrimSuffix(url, ".git")
	if i := strings.LastIndex(url, "/"); i >= 0 {
		return url[i+1:]
	}
	return url
}

func cloneOrPull(targetPath, repoURL string) error {
	gitDir := filepath.Join(targetPath, ".git")
	exists := false
	if fi, err := os.Stat(gitDir); err == nil && fi.IsDir() {
		exists = true
	}

	if exists {
		fmt.Fprintf(os.Stderr, "Pulling %s\n", targetPath)
		c := exec.Command("git", "pull")
		c.Dir = targetPath
		if !quiet {
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
		}
		if err := c.Run(); err != nil {
			return err
		}
		return nil
	}

	fmt.Fprintf(os.Stderr, "Cloning %s -> %s\n", repoURL, targetPath)
	c := exec.Command("git", "clone", repoURL, targetPath)
	if !quiet {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	}
	return c.Run()
}

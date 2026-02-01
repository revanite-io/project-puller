package main

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/revanite-io/project-puller/internal/load"
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
	username string
	ssh      bool
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
	rootCmd.Flags().StringVarP(&username, "username", "u", "", "Fork username; clone with remote upstream and add your fork as origin")
	rootCmd.Flags().BoolVar(&ssh, "ssh", false, "Use SSH URLs for clone and remotes (default: HTTPS)")
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
		effectiveURL, err := normalizeRepoURL(repoURL, ssh)
		if err != nil {
			return fmt.Errorf("repo %s: %w", repoURL, err)
		}
		dirName := repoDirName(r, repoURL, usedNames)
		usedNames[dirName] = true
		targetPath := filepath.Join(dir, dirName)

		if err := cloneOrPull(targetPath, effectiveURL, username); err != nil {
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

// normalizeRepoURL returns the repo URL in SSH or HTTPS form depending on useSSH.
func normalizeRepoURL(repoURL string, useSSH bool) (string, error) {
	if useSSH {
		return repoURLToSSH(repoURL)
	}
	return repoURLToHTTPS(repoURL)
}

func repoURLToSSH(repoURL string) (string, error) {
	repoURL = strings.TrimSpace(repoURL)
	// Already SSH (git@host:path or host:path)
	if strings.HasPrefix(repoURL, "git@") {
		return repoURL, nil
	}
	if idx := strings.Index(repoURL, ":"); idx > 0 && !strings.Contains(repoURL[:idx], "/") && !strings.HasPrefix(repoURL, "http") {
		return repoURL, nil
	}
	// GitHub HTTPS -> SSH
	if strings.HasPrefix(repoURL, "https://github.com/") || strings.HasPrefix(repoURL, "http://github.com/") {
		u, err := url.Parse(repoURL)
		if err != nil {
			return "", fmt.Errorf("invalid GitHub URL: %w", err)
		}
		path := strings.Trim(u.Path, "/")
		path = strings.TrimSuffix(path, ".git")
		if path == "" || !strings.Contains(path, "/") {
			return "", fmt.Errorf("GitHub URL has no owner/repo path: %s", repoURL)
		}
		return "git@github.com:" + path + ".git", nil
	}
	// Generic HTTPS -> SSH (https://host/owner/repo -> git@host:owner/repo.git)
	if strings.HasPrefix(repoURL, "https://") || strings.HasPrefix(repoURL, "http://") {
		u, err := url.Parse(repoURL)
		if err != nil {
			return "", fmt.Errorf("invalid URL: %w", err)
		}
		path := strings.Trim(u.Path, "/")
		path = strings.TrimSuffix(path, ".git")
		if path == "" || !strings.Contains(path, "/") {
			return "", fmt.Errorf("URL has no owner/repo path: %s", repoURL)
		}
		return "git@" + u.Host + ":" + path + ".git", nil
	}
	return "", fmt.Errorf("cannot convert to SSH: %s", repoURL)
}

func repoURLToHTTPS(repoURL string) (string, error) {
	repoURL = strings.TrimSpace(repoURL)
	// Already HTTPS
	if strings.HasPrefix(repoURL, "https://") || strings.HasPrefix(repoURL, "http://") {
		return repoURL, nil
	}
	// GitHub SSH -> HTTPS (git@github.com:owner/repo[.git] -> https://github.com/owner/repo)
	if strings.HasPrefix(repoURL, "git@github.com:") {
		path := strings.TrimPrefix(repoURL, "git@github.com:")
		path = strings.TrimSuffix(path, ".git")
		if path == "" || !strings.Contains(path, "/") {
			return "", fmt.Errorf("GitHub SSH URL has no owner/repo path: %s", repoURL)
		}
		return "https://github.com/" + path, nil
	}
	// Generic SSH (host:owner/repo or git@host:owner/repo) -> HTTPS
	if strings.HasPrefix(repoURL, "git@") {
		rest := repoURL[len("git@"):]
		idx := strings.Index(rest, ":")
		if idx <= 0 {
			return "", fmt.Errorf("SSH URL has no host:path: %s", repoURL)
		}
		host, path := rest[:idx], rest[idx+1:]
		path = strings.TrimSuffix(path, ".git")
		return "https://" + host + "/" + path, nil
	}
	if idx := strings.Index(repoURL, ":"); idx > 0 && !strings.Contains(repoURL[:idx], "/") {
		host, path := repoURL[:idx], repoURL[idx+1:]
		path = strings.TrimSuffix(path, ".git")
		return "https://" + host + "/" + path, nil
	}
	return "", fmt.Errorf("cannot convert to HTTPS: %s", repoURL)
}

// forkURLFromUpstream returns the fork URL for the given username.
// Handles GitHub HTTPS, GitHub SSH, and generic host URLs (replaces first path segment with username).
func forkURLFromUpstream(repoURL, username string) (string, error) {
	repoURL = strings.TrimSpace(repoURL)
	username = strings.TrimSpace(username)
	if username == "" {
		return "", fmt.Errorf("username is empty")
	}
	// GitHub HTTPS: https://github.com/owner/repo[.git]
	if strings.HasPrefix(repoURL, "https://github.com/") || strings.HasPrefix(repoURL, "http://github.com/") {
		u, err := url.Parse(repoURL)
		if err != nil {
			return "", fmt.Errorf("invalid GitHub URL: %w", err)
		}
		path := strings.TrimPrefix(u.Path, "/")
		path = strings.TrimSuffix(path, ".git")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) < 2 {
			return "", fmt.Errorf("GitHub URL has no owner/repo path: %s", repoURL)
		}
		u.Path = "/" + username + "/" + parts[1]
		if strings.HasSuffix(repoURL, ".git") {
			u.Path += ".git"
		}
		return u.String(), nil
	}
	// GitHub SSH: git@github.com:owner/repo[.git]
	if strings.HasPrefix(repoURL, "git@github.com:") {
		rest := strings.TrimPrefix(repoURL, "git@github.com:")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) < 2 {
			return "", fmt.Errorf("GitHub SSH URL has no owner/repo path: %s", repoURL)
		}
		repo := strings.TrimSuffix(parts[1], ".git")
		return "git@github.com:" + username + "/" + repo + ".git", nil
	}
	// Generic HTTPS: replace first path segment with username
	if strings.HasPrefix(repoURL, "https://") || strings.HasPrefix(repoURL, "http://") {
		u, err := url.Parse(repoURL)
		if err != nil {
			return "", fmt.Errorf("invalid URL: %w", err)
		}
		path := strings.Trim(u.Path, "/")
		segments := strings.SplitN(path, "/", 2)
		if len(segments) < 2 {
			return "", fmt.Errorf("URL has no owner/repo path: %s", repoURL)
		}
		u.Path = "/" + username + "/" + segments[1]
		return u.String(), nil
	}
	// Generic SSH: host:owner/repo -> host:username/repo
	if idx := strings.Index(repoURL, ":"); idx > 0 && !strings.Contains(repoURL[:idx], "/") {
		host := repoURL[:idx]
		rest := repoURL[idx+1:]
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) < 2 {
			return "", fmt.Errorf("SSH URL has no owner/repo path: %s", repoURL)
		}
		return host + ":" + username + "/" + parts[1], nil
	}
	return "", fmt.Errorf("cannot derive fork URL from: %s", repoURL)
}

func cloneOrPull(targetPath, repoURL, username string) error {
	gitDir := filepath.Join(targetPath, ".git")
	exists := false
	if fi, err := os.Stat(gitDir); err == nil && fi.IsDir() {
		exists = true
	}

	if exists {
		fmt.Fprintf(os.Stderr, "Pulling %s\n", targetPath)
		if username != "" {
			if err := ensureUpstreamOriginRemotes(targetPath, repoURL, username); err != nil {
				return err
			}
			// Pull from upstream (branch tracks upstream when we cloned with -o upstream, or we renamed origin->upstream)
			return runGit(exec.Command("git", "pull", "upstream"), targetPath)
		}
		return runGit(exec.Command("git", "pull"), targetPath)
	}

	if username != "" {
		fmt.Fprintf(os.Stderr, "Cloning %s -> %s (upstream)\n", repoURL, targetPath)
		if err := runGit(exec.Command("git", "clone", "-o", "upstream", repoURL, targetPath), "."); err != nil {
			return err
		}
		forkURL, err := forkURLFromUpstream(repoURL, username)
		if err != nil {
			return err
		}
		return addOriginRemote(targetPath, forkURL)
	}

	fmt.Fprintf(os.Stderr, "Cloning %s -> %s\n", repoURL, targetPath)
	return runGit(exec.Command("git", "clone", repoURL, targetPath), ".")
}

// ensureUpstreamOriginRemotes ensures upstream (project) and origin (fork) exist; normalizes repos cloned without --username.
func ensureUpstreamOriginRemotes(targetPath, repoURL, username string) error {
	hasUpstream := remoteExists(targetPath, "upstream")
	hasOrigin := remoteExists(targetPath, "origin")

	if hasUpstream && !hasOrigin {
		forkURL, err := forkURLFromUpstream(repoURL, username)
		if err != nil {
			return err
		}
		return addOriginRemote(targetPath, forkURL)
	}
	if !hasUpstream && hasOrigin {
		// Repo was cloned without --username; origin is the project. Rename to upstream and add origin as fork.
		if err := runGit(exec.Command("git", "remote", "rename", "origin", "upstream"), targetPath); err != nil {
			return err
		}
		forkURL, err := forkURLFromUpstream(repoURL, username)
		if err != nil {
			return err
		}
		return addOriginRemote(targetPath, forkURL)
	}
	// Both exist or neither; if both exist we do nothing. If neither exists something is wrong; pull will fail.
	return nil
}

func remoteExists(dir, name string) bool {
	c := exec.Command("git", "remote", "get-url", name)
	c.Dir = dir
	c.Stdout = nil
	c.Stderr = nil
	return c.Run() == nil
}

// runGit runs cmd in dir, wiring stdout/stderr when !quiet.
func runGit(cmd *exec.Cmd, dir string) error {
	cmd.Dir = dir
	if !quiet {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

// addOriginRemote adds remote "origin" with url in the repo at targetPath.
func addOriginRemote(targetPath, url string) error {
	return runGit(exec.Command("git", "remote", "add", "origin", url), targetPath)
}

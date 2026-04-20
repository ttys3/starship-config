package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// locateGitDir walks up from startDir to find a .git directory or file
// (file case supports worktrees: contents begin with "gitdir: <path>").
// Returns the resolved .git directory path, or "" if not found.
//
// Note: for linked worktrees the returned path is the worktree-private
// git dir ($GIT_DIR), not the common dir. See resolveConfigPath.
func locateGitDir(startDir string) string {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, ".git")
		info, err := os.Lstat(candidate)
		if err == nil {
			if info.IsDir() {
				return candidate
			}
			// Worktree / submodule: .git is a file with "gitdir: <path>".
			if resolved := readGitdirFile(candidate); resolved != "" {
				return resolved
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func readGitdirFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "gitdir:"); ok {
			resolved := strings.TrimSpace(v)
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(path), resolved)
			}
			return filepath.Clean(resolved)
		}
	}
	return ""
}

// resolveConfigPath returns the path of the config file to read for a given
// gitDir. For a normal .git directory this is just <gitDir>/config. For a
// linked worktree's git dir (which contains a `commondir` file pointing at
// the main repo's .git), git stores `config` at $GIT_COMMON_DIR, not in the
// worktree-private dir, so reading <gitDir>/config would always miss.
func resolveConfigPath(gitDir string) string {
	if data, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		common := strings.TrimSpace(string(data))
		if common != "" {
			if !filepath.IsAbs(common) {
				common = filepath.Join(gitDir, common)
			}
			return filepath.Join(filepath.Clean(common), "config")
		}
	}
	return filepath.Join(gitDir, "config")
}

// parseGitConfigRemoteURL reads the config file under gitDir (resolving
// $GIT_COMMON_DIR for worktrees) and returns the url of [remote "origin"].
// Returns "" if the section or key is missing.
//
// Hand-rolled parser instead of exec'ing `git` to keep the hot path <10ms.
func parseGitConfigRemoteURL(gitDir string) string {
	f, err := os.Open(resolveConfigPath(gitDir))
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inOrigin := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inOrigin = isRemoteOriginSection(line)
			continue
		}
		if !inOrigin {
			continue
		}
		key, val, ok := splitKV(line)
		if !ok {
			continue
		}
		if key == "url" {
			return val
		}
	}
	return ""
}

// isRemoteOriginSection checks whether a section header names the origin
// remote. Section names are case-insensitive per git-config(5); subsection
// names (here "origin") are case-sensitive.
func isRemoteOriginSection(header string) bool {
	if len(header) < 2 || header[0] != '[' || header[len(header)-1] != ']' {
		return false
	}
	inner := strings.TrimSpace(header[1 : len(header)-1])
	if len(inner) < len("remote") {
		return false
	}
	if !strings.EqualFold(inner[:len("remote")], "remote") {
		return false
	}
	rest := strings.TrimSpace(inner[len("remote"):])
	return rest == `"origin"`
}

func splitKV(line string) (key, val string, ok bool) {
	k, v, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(k)
	val = strings.TrimSpace(v)
	// Strip inline comment: "... # comment" or "... ; comment" if preceded by whitespace.
	val = stripInlineComment(val)
	// Strip surrounding quotes (git allows quoted values).
	val = strings.Trim(val, `"`)
	return key, val, key != ""
}

func stripInlineComment(s string) string {
	for i := 0; i < len(s); i++ {
		if (s[i] == '#' || s[i] == ';') && i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
			return strings.TrimRight(s[:i], " \t")
		}
	}
	return s
}

// githubURLRegex anchors github.com at the URL's authority position so that
// hostile URLs like "ssh://git@evil.com/github.com/foo/bar.git" are rejected
// (the path segment github.com is not the host). The allowed forms:
//
//	git@github.com:<owner>/<repo>(.git)?
//	(https|http|ssh|git)://[user[:pw]@]github.com[:/]<owner>/<repo>(.git)?
var githubURLRegex = regexp.MustCompile(
	`^(?:git@github\.com:|(?:https?|ssh|git)://(?:[^@/\s]*@)?github\.com[:/])([^/:\s]+)/([^/\s]+?)(?:\.git)?/?$`)

// isValidGitHubName enforces a conservative character whitelist for owner
// and repo names. GitHub's own rules allow only [A-Za-z0-9._-] plus some
// length limits (39 for user, 100 for repo) — we reject anything outside
// that set to prevent:
//   - path traversal when constructing cache file names (../, ./)
//   - gh CLI flag injection when the name is passed as an argument (an
//     owner that starts with "-" would be parsed as a flag by gh)
//   - URL smuggling / null bytes bypassing the \s class in the URL regex
//
// The first character must be alphanumeric: this blocks "-flag", ".hidden"
// and "..traversal" forms at the position where they matter.
func isValidGitHubName(s string) bool {
	if s == "" {
		return false
	}
	if !isAlphanum(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if isAlphanum(c) || c == '.' || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func isAlphanum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// parseGitHubOwnerRepo extracts owner/repo from a git remote URL targeting
// github.com. Returns "" for non-GitHub URLs (GHES, GitLab, Bitbucket, etc.),
// and also rejects any owner/repo that contains characters outside the
// GitHub allowlist (see isValidGitHubName).
// Supports: git@github.com:o/r(.git)?, https://github.com/o/r(.git)?,
// ssh://git@github.com/o/r, git://github.com/o/r.
func parseGitHubOwnerRepo(url string) (owner, repo string) {
	url = strings.TrimSpace(url)
	if url == "" {
		return "", ""
	}
	m := githubURLRegex.FindStringSubmatch(url)
	if m == nil {
		return "", ""
	}
	if !isValidGitHubName(m[1]) || !isValidGitHubName(m[2]) {
		return "", ""
	}
	return m[1], m[2]
}

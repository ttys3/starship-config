package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func main() {
	var (
		refresh string
		clear   bool
		status  bool
	)
	flag.StringVar(&refresh, "refresh", "", "internal: refresh cache for owner/repo (detached child)")
	flag.BoolVar(&clear, "clear", false, "clear cache directory")
	flag.BoolVar(&status, "status", false, "print human-readable diagnostic for the current repo")
	flag.Parse()

	switch {
	case clear:
		if err := runClear(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case status:
		if err := runStatus(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case refresh != "":
		if err := runRefresh(refresh); err != nil {
			os.Exit(1)
		}
	default:
		runPrompt()
	}
}

// runStatus is the user-facing diagnostic subcommand. It prints the current
// repo's resolved owner/repo, the cached visibility state, age, last error,
// and — when the state is anon or an error is pending — an actionable hint
// showing the exact commands to recover.
func runStatus() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	gitDir := locateGitDir(cwd)
	if gitDir == "" {
		fmt.Println("not inside a git repository")
		return nil
	}
	url := parseGitConfigRemoteURL(gitDir)
	if url == "" {
		fmt.Println("no origin remote configured")
		return nil
	}
	owner, repo := parseGitHubOwnerRepo(url)
	if owner == "" || repo == "" {
		fmt.Printf("remote URL is not github.com: %s\n", url)
		fmt.Println("visibility icon only supports github.com remotes")
		return nil
	}

	fmt.Printf("Repository: %s/%s\n", owner, repo)
	fmt.Printf("Remote URL: %s\n", url)

	path, err := cachePath(owner, repo)
	if err != nil {
		return err
	}
	fmt.Printf("Cache file: %s\n", path)

	c, err := readCache(path)
	if err != nil {
		fmt.Printf("Cache read error: %v\n", err)
		return nil
	}
	if c == nil {
		fmt.Println("Cache state: (none yet)")
		fmt.Println()
		fmt.Println(anonHint)
		return nil
	}

	state := "unknown ❓"
	if c.Source == sourceGH || c.Source == sourceAPI {
		if c.Private {
			state = "private 🔒"
		} else {
			state = "public 👀"
		}
	}
	fmt.Printf("State:      %s\n", state)
	fmt.Printf("Source:     %s\n", c.Source)
	fmt.Printf("Updated:    %s (%s ago)\n",
		c.UpdatedAt.Format(time.RFC3339),
		time.Since(c.UpdatedAt).Round(time.Second))
	if c.ETag != "" {
		fmt.Printf("ETag:       %s\n", c.ETag)
	}
	if !c.LastErrorAt.IsZero() {
		fmt.Printf("LastError:  %s (at %s)\n",
			c.LastError, c.LastErrorAt.Format(time.RFC3339))
	}
	if c.Source == sourceAnon || (c.LastError != "" && c.LastErrorAt.After(c.UpdatedAt.Add(-time.Second))) {
		fmt.Println()
		fmt.Println(anonHint)
	}
	return nil
}

// runPrompt is the hot path executed on every starship prompt render.
// Invariant: must be silent on errors and finish within a few ms on a
// warm cache. Any expensive work (network, gh exec) is forked into a
// detached --refresh child and never awaited.
func runPrompt() {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	gitDir := locateGitDir(cwd)
	if gitDir == "" {
		return
	}
	url := parseGitConfigRemoteURL(gitDir)
	if url == "" {
		return
	}
	owner, repo := parseGitHubOwnerRepo(url)
	if owner == "" || repo == "" {
		return
	}

	path, err := cachePath(owner, repo)
	if err != nil {
		return
	}
	c, _ := readCache(path)

	cfg := loadTTLConfig()
	if shouldRefresh(c, time.Now(), cfg) {
		forkRefresh(owner, repo)
	}

	fmt.Print(decideOutput(c))
}

// runClear wipes the entire cache directory. Best-effort; ignores ENOENT.
func runClear() error {
	dir, err := cacheDir()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// forkRefresh spawns a detached `self --refresh <owner>/<repo>` child and
// returns immediately without waiting. Setsid detaches the child from the
// parent's process group and session, ensuring it survives shell exit.
func forkRefresh(owner, repo string) {
	self, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(self, "--refresh", owner+"/"+repo)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	_ = cmd.Start()
	// Do NOT Wait; intentionally release the child.
}

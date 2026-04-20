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
	)
	flag.StringVar(&refresh, "refresh", "", "internal: refresh cache for owner/repo (detached child)")
	flag.BoolVar(&clear, "clear", false, "clear cache directory")
	flag.Parse()

	switch {
	case clear:
		if err := runClear(); err != nil {
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

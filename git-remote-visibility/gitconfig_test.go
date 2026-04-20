package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGitHubOwnerRepo(t *testing.T) {
	cases := []struct {
		url, owner, repo string
	}{
		{"git@github.com:ttys3/starship-config.git", "ttys3", "starship-config"},
		{"git@github.com:ttys3/starship-config", "ttys3", "starship-config"},
		{"https://github.com/ttys3/starship-config.git", "ttys3", "starship-config"},
		{"https://github.com/ttys3/starship-config", "ttys3", "starship-config"},
		{"https://github.com/ttys3/starship-config/", "ttys3", "starship-config"},
		{"ssh://git@github.com/ttys3/starship-config", "ttys3", "starship-config"},
		{"ssh://git@github.com/ttys3/starship-config.git", "ttys3", "starship-config"},
		{"git://github.com/ttys3/starship-config.git", "ttys3", "starship-config"},
		{"https://user:token@github.com/ttys3/starship-config.git", "ttys3", "starship-config"},

		// Not GitHub — must reject.
		{"git@github.enterprise.com:ttys3/repo.git", "", ""},
		{"https://github.enterprise.com/ttys3/repo.git", "", ""},
		{"git@gitlab.com:ttys3/repo.git", "", ""},
		{"https://gitlab.com/ttys3/repo.git", "", ""},
		{"https://bitbucket.org/ttys3/repo.git", "", ""},
		{"", "", ""},
		{"   ", "", ""},
		{"not a url at all", "", ""},

		// Host-confusion attacks: github.com embedded in path of another host.
		{"ssh://git@evil.com/github.com/foo/bar.git", "", ""},
		{"https://evil.com/mirror/github.com/foo/bar.git", "", ""},
		{"https://evil.com/github.com/foo/bar", "", ""},

		// Owner/repo character allowlist — must reject traversal, flag injection, nulls.
		{"git@github.com:../foo/repo.git", "", ""},
		{"git@github.com:foo/../repo.git", "", ""},
		{"git@github.com:../..git", "", ""},
		{"git@github.com:-rf/foo.git", "", ""},
		{"git@github.com:foo/-rf.git", "", ""},
		{"https://github.com/foo\x00bar/repo.git", "", ""},
		{"https://github.com/./repo.git", "", ""},
		{"https://github.com/foo/.", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			o, r := parseGitHubOwnerRepo(tc.url)
			if o != tc.owner || r != tc.repo {
				t.Fatalf("parseGitHubOwnerRepo(%q) = (%q, %q); want (%q, %q)",
					tc.url, o, r, tc.owner, tc.repo)
			}
		})
	}
}

func TestParseGitConfigRemoteURL(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "standard origin",
			content: `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = https://github.com/a/b.git
	fetch = +refs/heads/*:refs/remotes/origin/*
`,
			want: "https://github.com/a/b.git",
		},
		{
			name: "origin after other remote",
			content: `[remote "upstream"]
	url = https://github.com/up/stream.git
[remote "origin"]
	url = git@github.com:a/b.git
`,
			want: "git@github.com:a/b.git",
		},
		{
			name: "no origin",
			content: `[remote "upstream"]
	url = https://github.com/up/stream.git
`,
			want: "",
		},
		{
			name:    "empty file",
			content: "",
			want:    "",
		},
		{
			name: "with comments and blank lines",
			content: `# header comment
; semicolon comment

[remote "origin"]
	# inside comment
	url = https://github.com/a/b.git  # trailing
`,
			want: "https://github.com/a/b.git",
		},
		{
			name:    "crlf line endings",
			content: "[remote \"origin\"]\r\n\turl = https://github.com/a/b.git\r\n",
			want:    "https://github.com/a/b.git",
		},
		{
			name: "quoted value",
			content: `[remote "origin"]
	url = "https://github.com/a/b.git"
`,
			want: "https://github.com/a/b.git",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "config"), []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			got := parseGitConfigRemoteURL(dir)
			if got != tc.want {
				t.Fatalf("parseGitConfigRemoteURL: got %q; want %q", got, tc.want)
			}
		})
	}
}

func TestLocateGitDir(t *testing.T) {
	// Layout: root/.git (directory), root/sub/nested
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "sub", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := locateGitDir(nested); got != gitDir {
		t.Errorf("from nested: got %q; want %q", got, gitDir)
	}
	if got := locateGitDir(root); got != gitDir {
		t.Errorf("from root: got %q; want %q", got, gitDir)
	}

	// Unrelated dir (no .git above) should return empty.
	orphan := t.TempDir()
	if got := locateGitDir(orphan); got != "" {
		t.Errorf("orphan: got %q; want empty", got)
	}
}

func TestLocateGitDirWorktree(t *testing.T) {
	// Worktree layout: main repo has real .git dir; worktree/.git is a FILE
	// with "gitdir: <abs path to main .git/worktrees/foo>".
	root := t.TempDir()
	mainGit := filepath.Join(root, "main", ".git", "worktrees", "wt1")
	if err := os.MkdirAll(mainGit, 0o755); err != nil {
		t.Fatal(err)
	}
	wtRoot := filepath.Join(root, "wt")
	if err := os.MkdirAll(wtRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	gitFile := filepath.Join(wtRoot, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+mainGit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sub := filepath.Join(wtRoot, "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got := locateGitDir(sub)
	if got != mainGit {
		t.Errorf("worktree: got %q; want %q", got, mainGit)
	}
}

// TestParseGitConfigRemoteURLWorktree exercises the $GIT_COMMON_DIR resolution:
// a linked worktree's private git dir has `commondir` + no `config`; the real
// config lives in the main repo's .git. Without the commondir lookup the tool
// would see every worktree as "unknown".
func TestParseGitConfigRemoteURLWorktree(t *testing.T) {
	root := t.TempDir()
	mainGit := filepath.Join(root, "main", ".git")
	if err := os.MkdirAll(mainGit, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mainGit, "config"),
		[]byte("[remote \"origin\"]\n\turl = git@github.com:a/b.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Worktree private dir with commondir pointing at the main .git.
	wtGit := filepath.Join(mainGit, "worktrees", "wt1")
	if err := os.MkdirAll(wtGit, 0o755); err != nil {
		t.Fatal(err)
	}
	// Absolute commondir path.
	if err := os.WriteFile(filepath.Join(wtGit, "commondir"), []byte(mainGit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := parseGitConfigRemoteURL(wtGit); got != "git@github.com:a/b.git" {
		t.Errorf("absolute commondir: got %q", got)
	}

	// Relative commondir ("../../" back to main .git from worktrees/wt1).
	if err := os.WriteFile(filepath.Join(wtGit, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := parseGitConfigRemoteURL(wtGit); got != "git@github.com:a/b.git" {
		t.Errorf("relative commondir: got %q", got)
	}
}

func TestReadGitdirFile(t *testing.T) {
	root := t.TempDir()

	// Absolute path.
	absTarget := filepath.Join(root, "target")
	if err := os.MkdirAll(absTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	absFile := filepath.Join(root, "gitfile_abs")
	if err := os.WriteFile(absFile, []byte("gitdir: "+absTarget+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readGitdirFile(absFile); got != absTarget {
		t.Errorf("abs: got %q; want %q", got, absTarget)
	}

	// Relative path (resolved against the gitfile's parent).
	relFile := filepath.Join(root, "relfile")
	if err := os.WriteFile(relFile, []byte("gitdir: target\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readGitdirFile(relFile); got != absTarget {
		t.Errorf("rel: got %q; want %q", got, absTarget)
	}

	// CRLF line endings.
	crlfFile := filepath.Join(root, "crlf")
	if err := os.WriteFile(crlfFile, []byte("gitdir: "+absTarget+"\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readGitdirFile(crlfFile); got != absTarget {
		t.Errorf("crlf: got %q; want %q", got, absTarget)
	}

	// Missing file.
	if got := readGitdirFile(filepath.Join(root, "nope")); got != "" {
		t.Errorf("missing: got %q; want empty", got)
	}

	// File without gitdir prefix.
	bogus := filepath.Join(root, "bogus")
	if err := os.WriteFile(bogus, []byte("some other content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readGitdirFile(bogus); got != "" {
		t.Errorf("bogus: got %q; want empty", got)
	}
}

func TestIsRemoteOriginSection(t *testing.T) {
	cases := []struct {
		header string
		want   bool
	}{
		{`[remote "origin"]`, true},
		{`[remote  "origin"]`, true}, // extra spaces
		{`[Remote "origin"]`, true},  // case-insensitive section name
		{`[REMOTE "origin"]`, true},
		{`[remote "upstream"]`, false},         // wrong subsection
		{`[remote "Origin"]`, false},           // subsection IS case-sensitive
		{`[core]`, false},                      // different section
		{`[branch "origin"]`, false},           // different section with same sub
		{`remote "origin"`, false},             // missing brackets
		{`[]`, false},
		{`[remote]`, false},                    // no subsection at all
	}
	for _, tc := range cases {
		t.Run(tc.header, func(t *testing.T) {
			if got := isRemoteOriginSection(tc.header); got != tc.want {
				t.Errorf("got %v; want %v", got, tc.want)
			}
		})
	}
}

func TestSplitKV(t *testing.T) {
	cases := []struct {
		line, key, val string
		ok             bool
	}{
		{"url = value", "url", "value", true},
		{"  url   =   value  ", "url", "value", true},
		{"url=value", "url", "value", true},
		{`url = "quoted value"`, "url", "quoted value", true},
		{"url = val # trailing comment", "url", "val", true},
		{"url = val ; trailing", "url", "val", true},
		{"nocomment", "", "", false},
		{"= onlyvalue", "", "onlyvalue", false},     // empty key rejected
		{"key = a = b", "key", "a = b", true},       // only first '=' splits
		{"key\t=\tvalue", "key", "value", true},
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			k, v, ok := splitKV(tc.line)
			if ok != tc.ok || k != tc.key || v != tc.val {
				t.Errorf("got (%q,%q,%v); want (%q,%q,%v)", k, v, ok, tc.key, tc.val, tc.ok)
			}
		})
	}
}

func TestStripInlineComment(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"val # comment", "val"},
		{"val ; comment", "val"},
		{"val\t# comment", "val"},
		{"#atstart", "#atstart"},               // no leading whitespace -> keep
		{";atstart", ";atstart"},
		{"val # one # two", "val"},
		{"val#nospace", "val#nospace"},         // # not preceded by ws -> keep
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := stripInlineComment(tc.in); got != tc.want {
				t.Errorf("got %q; want %q", got, tc.want)
			}
		})
	}
}

func TestIsValidGitHubName(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"ttys3", true},
		{"starship-config", true},
		{"foo.bar", true},
		{"a_b-c.1", true},
		{"A", true},
		{"9", true},
		{"", false},
		{".", false},
		{"..", false},
		{"foo/bar", false},
		{"foo\x00bar", false},
		{"foo bar", false},
		{"-foo", false},            // first char must be alphanumeric (blocks gh flag injection)
		{".foo", false},
		{"_foo", false},
		{"foo\n", false},
		{"%^&*", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := isValidGitHubName(tc.in); got != tc.ok {
				t.Errorf("isValidGitHubName(%q) = %v; want %v", tc.in, got, tc.ok)
			}
		})
	}
}

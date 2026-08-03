// Package repository associates a directory with the version-control
// repository that encloses it, and reads the small amount of repository
// metadata the safety model needs.
//
// The package stats directory entries and reads two things: the symbolic ref
// in .git/HEAD, and the "gitdir:" pointer in a .git file for worktrees and
// submodules. Both are a few dozen bytes of plumbing. No file inside the
// working tree is ever opened; ghostgc records paths and metadata, never
// contents.
package repository

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// maxWalkDepth bounds the upward search. Real repositories are nowhere near
// this deep; the bound exists so that a pathological path cannot spin.
const maxWalkDepth = 48

// maxCacheEntries bounds each memoisation table. When one fills, it is dropped
// wholesale rather than growing without limit.
const maxCacheEntries = 4096

// maxHeadBytes caps the read of .git/HEAD and of a .git pointer file. Both are
// short by construction; the cap means a file that is not what it claims to be
// cannot become a large allocation.
const maxHeadBytes = 4096

// infoTTL is how long repository metadata is reused. Branch and lock state
// change while ghostgc is watching, so this must be short, but re-reading them
// for every process on every scan would be pointless.
const infoTTL = 30 * time.Second

// Info is the repository metadata ghostgc keeps.
type Info struct {
	// Root is the repository root, or "" when the directory is not in one.
	Root string `json:"root,omitempty"`
	// Name is the last path element of Root.
	Name string `json:"name,omitempty"`
	// Branch is the checked-out branch, or "" when detached or unreadable.
	Branch string `json:"branch,omitempty"`
	// Detached reports a detached HEAD.
	Detached bool `json:"detached,omitempty"`
	// Locks lists git lock files present right now. A held lock means an
	// operation is in flight and the process holding it must not be disturbed.
	Locks []string `json:"locks,omitempty"`
}

// Busy reports whether a git operation appears to be in flight.
func (i Info) Busy() bool { return len(i.Locks) > 0 }

// lockFiles are the git lock files worth noticing. Their presence is what
// section 16 means by "holds a Git lock": a process interrupted while one of
// these exists can leave the repository in a state the user has to repair by
// hand.
var lockFiles = []string{
	"index.lock",
	"HEAD.lock",
	"config.lock",
	"packed-refs.lock",
	"MERGE_HEAD.lock",
	"ORIG_HEAD.lock",
	"shallow.lock",
}

type infoEntry struct {
	info Info
	at   time.Time
}

// Finder resolves repository roots and metadata with bounded caches.
type Finder struct {
	mu    sync.Mutex
	roots map[string]string
	infos map[string]infoEntry

	// now is overridable so cache expiry is testable.
	now func() time.Time
}

// NewFinder constructs a Finder.
func NewFinder() *Finder {
	return &Finder{
		roots: make(map[string]string),
		infos: make(map[string]infoEntry),
		now:   time.Now,
	}
}

// Root returns the repository root enclosing dir, or "" when dir is not inside
// a repository.
func (f *Finder) Root(dir string) string {
	if dir == "" || !filepath.IsAbs(dir) {
		return ""
	}
	f.mu.Lock()
	if root, ok := f.roots[dir]; ok {
		f.mu.Unlock()
		return root
	}
	f.mu.Unlock()

	root := findRoot(dir)

	f.mu.Lock()
	if len(f.roots) >= maxCacheEntries {
		f.roots = make(map[string]string)
	}
	f.roots[dir] = root
	f.mu.Unlock()
	return root
}

// Describe returns the repository metadata for the repository enclosing dir.
// The zero Info means dir is not inside a repository.
func (f *Finder) Describe(dir string) Info {
	root := f.Root(dir)
	if root == "" {
		return Info{}
	}

	f.mu.Lock()
	entry, ok := f.infos[root]
	fresh := ok && f.now().Sub(entry.at) < infoTTL
	f.mu.Unlock()
	if fresh {
		return entry.info
	}

	info := describe(root)

	f.mu.Lock()
	if len(f.infos) >= maxCacheEntries {
		f.infos = make(map[string]infoEntry)
	}
	f.infos[root] = infoEntry{info: info, at: f.now()}
	f.mu.Unlock()
	return info
}

// Name returns the last path element of a repository root, which is what the
// CLI displays.
func Name(root string) string {
	if root == "" {
		return ""
	}
	return filepath.Base(root)
}

func findRoot(dir string) string {
	cur := filepath.Clean(dir)
	for depth := 0; depth < maxWalkDepth; depth++ {
		// A .git directory marks a normal checkout; a .git file marks a
		// worktree or submodule. Either is a repository for our purposes.
		if _, err := os.Lstat(filepath.Join(cur, ".git")); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
	return ""
}

func describe(root string) Info {
	info := Info{Root: root, Name: filepath.Base(root)}
	gitDir := resolveGitDir(root)
	if gitDir == "" {
		return info
	}

	if head, err := readSmall(filepath.Join(gitDir, "HEAD")); err == nil {
		ref := strings.TrimSpace(string(head))
		switch {
		case strings.HasPrefix(ref, "ref: refs/heads/"):
			info.Branch = strings.TrimPrefix(ref, "ref: refs/heads/")
		case strings.HasPrefix(ref, "ref: "):
			info.Branch = strings.TrimPrefix(ref, "ref: ")
		case ref != "":
			// A bare object id: HEAD is detached. The id itself is not
			// recorded; it says nothing useful about safety.
			info.Detached = true
		}
	}

	for _, name := range lockFiles {
		if _, err := os.Lstat(filepath.Join(gitDir, name)); err == nil {
			info.Locks = append(info.Locks, name)
		}
	}
	return info
}

// resolveGitDir returns the real git directory for a repository root, following
// the "gitdir:" pointer that worktrees and submodules use.
func resolveGitDir(root string) string {
	path := filepath.Join(root, ".git")
	fi, err := os.Lstat(path)
	if err != nil {
		return ""
	}
	if fi.IsDir() {
		return path
	}

	body, err := readSmall(path)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(bytes.TrimPrefix(bytes.TrimSpace(body), []byte("gitdir:"))))
	if line == "" || line == strings.TrimSpace(string(body)) {
		return ""
	}
	if !filepath.IsAbs(line) {
		line = filepath.Join(root, line)
	}
	if fi, err := os.Stat(line); err != nil || !fi.IsDir() {
		return ""
	}
	return line
}

// readSmall reads at most maxHeadBytes from a plumbing file.
func readSmall(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, maxHeadBytes)
	n, err := f.Read(buf)
	if n > 0 {
		return buf[:n], nil
	}
	if err != nil {
		return nil, err
	}
	return nil, nil
}

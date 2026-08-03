package process

// maxTreeDepth bounds ancestor and descendant walks. A real process tree is
// shallow; anything deeper is a symptom of inconsistent data and must not be
// allowed to spin.
const maxTreeDepth = 128

// LinkState describes why a process is or is not connected to its claimed
// parent in a snapshot.
type LinkState string

const (
	// LinkIntact means the claimed parent exists and could plausibly have
	// created this process.
	LinkIntact LinkState = "intact"
	// LinkReparented means the claimed parent is the init process, which on
	// macOS and Linux is what a process is handed to when its real parent
	// exits. Reparented does not imply orphaned.
	LinkReparented LinkState = "reparented"
	// LinkImpossible means the claimed parent exists but started after this
	// process did. The PID was reused; the link is a coincidence and is not
	// believed.
	LinkImpossible LinkState = "impossible"
	// LinkMissing means the claimed parent is not present in the snapshot.
	LinkMissing LinkState = "missing"
	// LinkRoot means the process is the init process itself.
	LinkRoot LinkState = "root"
)

// InitPID is the PID of the init process on macOS (launchd) and Linux.
const InitPID = 1

// Tree is a parent/child view over a Snapshot with PID-reuse protection.
//
// A parent link is only believed when the claimed parent exists in the
// snapshot and started no later than the child. Believing a link on PID alone
// is exactly how a cleanup tool ends up attributing an unrelated process to an
// agent session, so the check is not optional.
type Tree struct {
	snap     *Snapshot
	children map[int][]int
	links    map[int]LinkState
}

// BuildTree reconstructs parent/child relationships from a snapshot.
func BuildTree(s *Snapshot) *Tree {
	t := &Tree{
		snap:     s,
		children: make(map[int][]int),
		links:    make(map[int]LinkState),
	}
	if s == nil {
		return t
	}
	for _, p := range s.Processes {
		t.links[p.PID] = classifyLink(s, p)
		if t.links[p.PID] == LinkIntact {
			t.children[p.PPID] = append(t.children[p.PPID], p.PID)
		}
	}
	return t
}

func classifyLink(s *Snapshot, p Process) LinkState {
	switch {
	case p.PID == InitPID:
		return LinkRoot
	case p.PPID == 0:
		return LinkMissing
	case p.PPID == p.PID:
		// Self-parenting is impossible; refuse to build a cycle from it.
		return LinkImpossible
	}
	parent, ok := s.ByPID(p.PPID)
	if !ok {
		if p.PPID == InitPID {
			return LinkReparented
		}
		return LinkMissing
	}
	if parent.StartTime.After(p.StartTime) {
		// The claimed parent is younger than the child. The original parent
		// exited and this PID was recycled.
		return LinkImpossible
	}
	if p.PPID == InitPID {
		return LinkReparented
	}
	return LinkIntact
}

// Link reports the parent-link state for a PID in this snapshot.
func (t *Tree) Link(pid int) LinkState {
	if st, ok := t.links[pid]; ok {
		return st
	}
	return LinkMissing
}

// Children returns the PIDs whose parent link to pid is intact.
func (t *Tree) Children(pid int) []int {
	out := make([]int, len(t.children[pid]))
	copy(out, t.children[pid])
	return out
}

// Descendants returns every PID reachable downward from pid through intact
// links, in breadth-first order. The walk is cycle-safe and depth-bounded.
func (t *Tree) Descendants(pid int) []int {
	seen := map[int]bool{pid: true}
	var out []int
	frontier := t.Children(pid)
	for depth := 0; depth < maxTreeDepth && len(frontier) > 0; depth++ {
		var next []int
		for _, child := range frontier {
			if seen[child] {
				continue
			}
			seen[child] = true
			out = append(out, child)
			next = append(next, t.children[child]...)
		}
		frontier = next
	}
	return out
}

// Ancestors returns the chain of PIDs above pid through intact or reparented
// links, nearest first. The walk stops at the first link that is not believable.
func (t *Tree) Ancestors(pid int) []int {
	seen := map[int]bool{pid: true}
	var out []int
	cur := pid
	for depth := 0; depth < maxTreeDepth; depth++ {
		p, ok := t.snap.ByPID(cur)
		if !ok {
			return out
		}
		if t.Link(cur) != LinkIntact {
			return out
		}
		if seen[p.PPID] {
			return out
		}
		seen[p.PPID] = true
		out = append(out, p.PPID)
		cur = p.PPID
	}
	return out
}

// IsDescendantOf reports whether pid is reachable downward from ancestor.
func (t *Tree) IsDescendantOf(pid, ancestor int) bool {
	for _, a := range t.Ancestors(pid) {
		if a == ancestor {
			return true
		}
	}
	return false
}

// Roots returns PIDs with no believable parent inside the snapshot.
func (t *Tree) Roots() []int {
	var out []int
	for _, p := range t.snap.Processes {
		if t.Link(p.PID) != LinkIntact {
			out = append(out, p.PID)
		}
	}
	return out
}

// Command fake-agent stands in for a coding-agent session root in the
// integration fixture.
//
// It is built to a file named "codex" so that the kernel records that path as
// the process image and proc_pidpath reports a basename of "codex", which is
// what the adapter's identity evidence keys on. A shell script with a shebang
// would report the interpreter instead, and a copy of a system binary is
// forcibly terminated on Apple Silicon because copying invalidates its code signature —
// hence a real, locally built executable.
//
// It spawns a small tree and then waits. It signals nothing, ever.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--exit" {
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "--tick" {
		for {
			if err := os.WriteFile(os.Args[2], []byte(time.Now().Format(time.RFC3339Nano)), 0o600); err != nil {
				fmt.Fprintln(os.Stderr, "fake-agent tick:", err)
			}
			time.Sleep(2 * time.Second)
		}
	}
	// --sleep makes this binary stand in for a long-lived helper. The fixture
	// uses it for the detached child so that the child's environment is
	// readable: macOS withholds the environment of system binaries such as
	// /bin/sleep, which would make environment-derived membership untestable.
	if len(os.Args) > 1 && os.Args[1] == "--sleep" {
		// Not select{}: Go's runtime reports a program with no runnable
		// goroutines as a deadlock and exits.
		time.Sleep(time.Hour)
		return
	}

	repo := "."
	if len(os.Args) > 1 {
		repo = os.Args[1]
	}
	self, err := os.Executable()
	if err != nil {
		self = os.Args[0]
	}
	// Deliberately a differently named binary: see fixture-agent.sh.
	helper := filepath.Join(filepath.Dir(self), "fixture-helper")
	// Leave one exited child deliberately unreaped. This gives the live
	// classifier a fixture-owned zombie whose crashed state is immediate and
	// deterministic; teardown ends the parent that owns the process table entry.
	crashed := exec.Command(helper, "--exit")
	crashed.Dir = repo
	crashed.Stdout, crashed.Stderr = os.Stdout, os.Stderr
	if err := crashed.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "fake-agent crashed fixture:", err)
		os.Exit(1)
	}
	fmt.Printf("fixture crashed-child %d\n", crashed.Process.Pid)

	// A direct, non-broad helper gives cleanup tests one controlled process that
	// can become detached while remaining active. Teardown owns its recorded PID.
	candidate := exec.Command(helper, "--tick", filepath.Join(repo, "candidate.log"))
	candidate.Dir = repo
	candidate.Stdout, candidate.Stderr = os.Stdout, os.Stderr
	if err := candidate.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "fake-agent candidate:", err)
		os.Exit(1)
	}
	fmt.Printf("fixture candidate-child %d\n", candidate.Process.Pid)

	// A direct known-idle helper is the sole live action target. Because the
	// root is observed as its parent before the root exits, later reparenting is
	// evidence-backed; after five idle minutes it can become orphaned.
	action := exec.Command(helper, "--sleep")
	action.Dir = repo
	action.Stdout, action.Stderr = os.Stdout, os.Stderr
	if err := action.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "fake-agent action target:", err)
		os.Exit(1)
	}
	fmt.Printf("fixture action-child %d\n", action.Process.Pid)

	// A child shell that owns the rest of the tree, mirroring how an agent
	// shells out to do work.
	script := fmt.Sprintf(`
		sleep 3600 &
		echo "fixture idle-child $!"

		( while :; do echo tick >> %q; sleep 2; done ) &
		echo "fixture active-child $!"

		# A grandchild whose intermediate parent exits immediately, so the
		# kernel reparents it to launchd. Detached is not orphaned; the daemon
		# should still be able to trace it back to this session.
		#
		# The subshell backgrounds the child and then exits without waiting,
		# which orphans it. setsid would be the obvious tool and does not exist
		# on macOS.
		( %q --sleep >/dev/null 2>&1 & echo "fixture detached-child $!" ) &

		wait
	`, filepath.Join(repo, "build.log"), helper)

	child := exec.Command("/bin/sh", "-c", script)
	child.Dir = repo
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "fake-agent:", err)
		os.Exit(1)
	}
	fmt.Printf("fixture worker-shell %d\n", child.Process.Pid)

	// Exit on SIGTERM so the fixture's own teardown is clean.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	<-ch
	os.Exit(0)
}

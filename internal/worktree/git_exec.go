package worktree

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func (g *Git) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	if err := g.VerifyIdentity(); err != nil {
		return nil, err
	}
	if g.beforeExec != nil {
		g.beforeExec()
	}
	base := []string{"-c", "color.ui=false", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false"}
	if dir != "" {
		base = append(base, "-C", dir)
	}
	cmd := exec.CommandContext(ctx, g.execPath, append(base, args...)...)
	cmd.Env = gitEnvironment(os.Environ())
	out := &boundedBuffer{limit: g.maxBytes}
	errOut := &boundedBuffer{limit: 32 << 10}
	cmd.Stdout, cmd.Stderr = out, errOut
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("worktree: git command timed out: %w", ctx.Err())
	}
	if out.overflow || errOut.overflow {
		return nil, errors.New("worktree: git command exceeded its output bound")
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("worktree: git command exited %d", exit.ExitCode())
		}
		return nil, fmt.Errorf("worktree: starting private git execution snapshot: %w", err)
	}
	return out.Bytes(), nil
}

func gitEnvironment(environ []string) []string {
	clean := make([]string, 0, len(environ)+4)
	for _, item := range environ {
		key, _, found := strings.Cut(item, "=")
		if !found || strings.HasPrefix(key, "GIT_") {
			continue
		}
		clean = append(clean, item)
	}
	return append(clean,
		"GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "PAGER=cat", "GIT_OPTIONAL_LOCKS=0")
}

type boundedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.overflow = true
		return n, nil
	}
	if len(p) > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

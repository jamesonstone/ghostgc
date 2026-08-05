package platform_test

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jamesonstone/ghostgc/internal/platform"
	"github.com/jamesonstone/ghostgc/internal/process"
)

func TestPlatformRejectsNonTERMAndChangedIdentity(t *testing.T) {
	p, err := platform.New(platform.Options{})
	if err != nil {
		t.Skipf("no platform implementation for this host: %v", err)
	}
	invalid := process.Key{PID: os.Getpid(), StartTimeNs: 1}
	invalidExecutable := process.ExecutableIdentity{ExecPath: "/invalid", Comm: "invalid"}
	if err := p.SignalProcess(context.Background(), invalid, invalidExecutable, platform.Signal(-1)); !errors.Is(err, platform.ErrSignalNotAllowed) {
		t.Fatalf("non-TERM signal returned %v, want ErrSignalNotAllowed", err)
	}
	if err := p.SignalProcess(context.Background(), invalid, invalidExecutable, platform.SIGTERM); err == nil {
		t.Fatal("changed exact identity was accepted")
	}
}

func TestSignalPrimitiveIsSingleLiteralSIGTERM(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	allowed := filepath.Join(root, "internal", "platform", "darwin", "signal.go")
	var sites int
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || path == filepath.Join(root, "internal", "platform", "signal_gate_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		scanned := text
		if path == allowed {
			scanned = strings.Replace(scanned, "syscall.Kill(key.PID, syscall.SIGTERM)", "", 1)
		}
		scanned = strings.ReplaceAll(scanned, "platform.Signal(", "")
		for _, forbidden := range []string{"SIGKILL", "unix.Kill", "SYS_KILL", ".Kill(", ".Signal(", `"kill"`, `"pkill"`, `"killall"`} {
			if strings.Contains(scanned, forbidden) {
				t.Errorf("%s references forbidden terminator %q", path, forbidden)
			}
		}
		if count := strings.Count(text, "syscall.Kill"); count > 0 {
			sites += count
			if path != allowed || count != 1 || strings.Count(text, "syscall.Kill(key.PID, syscall.SIGTERM)") != 1 {
				t.Errorf("%s has a non-literal or unauthorized signal site", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if sites != 1 {
		t.Fatalf("found %d signalling sites, want exactly one", sites)
	}
}

func TestNoSourceFileShellsOutToATerminator(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "vendor") {
			return fs.SkipDir
		}
		if d.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(body), "\n") {
			if !strings.Contains(line, "exec.Command") {
				continue
			}
			for _, bad := range []string{`"kill"`, `"pkill"`, `"killall"`, `"launchctl", "kill"`} {
				if strings.Contains(line, bad) {
					t.Errorf("%s shells out to a terminator: %s", path, strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIrreversibleFilesystemCapabilitiesAreForegroundOnly(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cacheAllowed := filepath.Join(root, "internal", "cachefs", "purge_unix.go")
	worktreeAllowed := filepath.Join(root, "internal", "worktree", "finalizer.go")
	var cacheUnlinkSites, approvedLinkUnlinkSites, worktreeRemoveSites int
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "vendor" || d.Name() == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, _ := call.Fun.(*ast.SelectorExpr)
			if selector != nil {
				switch selector.Sel.Name {
				case "RemoveAll", "Unlink":
					t.Errorf("%s contains forbidden recursive or unanchored deletion call %s", path, selector.Sel.Name)
				case "Unlinkat":
					switch path {
					case cacheAllowed:
						cacheUnlinkSites++
					case worktreeAllowed:
						approvedLinkUnlinkSites++
					default:
						t.Errorf("%s contains unlink outside a foreground finalizer", path)
					}
				case "Purge", "Finalize":
					if strings.Contains(path, filepath.Join("internal", "daemon")) {
						t.Errorf("%s gives the daemon an irreversible executor call", path)
					}
				case "Remove":
					if strings.Contains(path, filepath.Join("internal", "daemon")) {
						t.Errorf("%s gives the daemon a direct filesystem removal call", path)
					}
				}
			}
			literals := stringArguments(call.Args)
			if shellsOutToDelete(selector, literals) {
				t.Errorf("%s shells out to filesystem deletion", path)
			}
			if adjacent(literals, "worktree", "remove") {
				worktreeRemoveSites++
				if path != worktreeAllowed {
					t.Errorf("%s invokes native worktree removal outside the foreground finalizer", path)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if cacheUnlinkSites != 1 {
		t.Fatalf("found %d production cache unlink sites, want exactly one", cacheUnlinkSites)
	}
	if approvedLinkUnlinkSites != 1 {
		t.Fatalf("found %d approved-link unlink sites, want exactly one", approvedLinkUnlinkSites)
	}
	if worktreeRemoveSites != 1 {
		t.Fatalf("found %d native worktree removal sites, want exactly one", worktreeRemoveSites)
	}
}

func stringArguments(args []ast.Expr) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		literal, ok := arg.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			continue
		}
		if value, err := strconv.Unquote(literal.Value); err == nil {
			out[i] = value
		}
	}
	return out
}

func adjacent(values []string, first, second string) bool {
	for i := 0; i+1 < len(values); i++ {
		if values[i] == first && values[i+1] == second {
			return true
		}
	}
	return false
}

func shellsOutToDelete(selector *ast.SelectorExpr, args []string) bool {
	if selector == nil || (selector.Sel.Name != "Command" && selector.Sel.Name != "CommandContext") {
		return false
	}
	for i, value := range args {
		if filepath.Base(value) == "rm" || (i > 0 && strings.Contains(" "+value+" ", " rm ")) {
			return true
		}
	}
	return false
}

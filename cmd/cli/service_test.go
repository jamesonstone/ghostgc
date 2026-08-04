package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/platform/platformtest"
)

func TestResolveSelfBinaryCanonicalizesSymlink(t *testing.T) {
	target, link := executableFixture(t)
	stubExecutablePath(t, link)

	got, err := resolveSelfBinary()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("resolveSelfBinary() = %q, want %q", got, want)
	}
}

func TestServiceInstallRegistersCurrentBinaryAsDaemon(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	target, link := executableFixture(t)
	stubExecutablePath(t, link)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	fake := platformtest.New(501)
	if err := serviceInstall(context.Background(), &env{paths: paths}, fake, nil); err != nil {
		t.Fatal(err)
	}

	canonical, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	opts := fake.InstalledService()
	if opts.BinaryPath != canonical {
		t.Errorf("binary = %q, want %q", opts.BinaryPath, canonical)
	}
	wantArgs := []string{"daemon", "--config", paths.Config}
	if !reflect.DeepEqual(opts.Arguments, wantArgs) {
		t.Errorf("arguments = %#v, want %#v", opts.Arguments, wantArgs)
	}
	if _, err := os.Stat(paths.Config); err != nil {
		t.Errorf("default config was not created: %v", err)
	}
}

func TestRunRoutesDaemonVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if code := run(context.Background(), []string{"daemon", "--version"}); code != 0 {
		t.Fatalf("run daemon version exit code = %d, want 0", code)
	}
}

func TestServiceInstallRejectsRemovedBinaryOption(t *testing.T) {
	fake := platformtest.New(501)
	err := serviceInstall(context.Background(), &env{}, fake, []string{"--binary", "/tmp/ghostgcd"})
	if err == nil {
		t.Fatal("service install accepted the removed --binary option")
	}
}

func executableFixture(t *testing.T) (target, link string) {
	t.Helper()
	dir := t.TempDir()
	target = filepath.Join(dir, "ghostgc-real")
	if err := os.WriteFile(target, []byte("test executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	link = filepath.Join(dir, "ghostgc")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	return target, link
}

func stubExecutablePath(t *testing.T, path string) {
	t.Helper()
	previous := executablePath
	executablePath = func() (string, error) { return path, nil }
	t.Cleanup(func() { executablePath = previous })
}

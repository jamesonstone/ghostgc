package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/platform"
	"github.com/jamesonstone/ghostgc/internal/platform/platformtest"
	"github.com/jamesonstone/ghostgc/internal/version"
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
	legacy := seedLegacyDaemon(t, link)

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
	if _, err := os.Lstat(legacy); !os.IsNotExist(err) {
		t.Errorf("legacy executable still exists after migration: %v", err)
	}
}

func TestServiceInstallPreservesLegacyExecutableWhenRegistrationFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, link := executableFixture(t)
	stubExecutablePath(t, link)
	legacy := seedLegacyDaemon(t, link)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	fake := installFailurePlatform{Platform: platformtest.New(501)}
	if err := serviceInstall(context.Background(), &env{paths: paths}, fake, nil); err == nil {
		t.Fatal("service install succeeded with a failing platform")
	}
	if _, err := os.Lstat(legacy); err != nil {
		t.Fatalf("legacy executable was removed before successful migration: %v", err)
	}
}

func TestVersionCommandsReportBuildOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, args := range [][]string{{"version"}, {"daemon", "--version"}} {
		output, code := captureStdout(t, func() int {
			return run(context.Background(), args)
		})
		if code != 0 {
			t.Fatalf("run %v exit code = %d, want 0", args, code)
		}
		want := "ghostgc " + version.String() + "\n"
		if output != want {
			t.Fatalf("run %v output = %q, want %q", args, output, want)
		}
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

func seedLegacyDaemon(t *testing.T, executable string) string {
	t.Helper()
	legacy := filepath.Join(filepath.Dir(executable), "ghostgcd")
	if err := os.WriteFile(legacy, []byte("legacy executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	return legacy
}

func stubExecutablePath(t *testing.T, path string) {
	t.Helper()
	previous := executablePath
	executablePath = func() (string, error) { return path, nil }
	t.Cleanup(func() { executablePath = previous })
}

func captureStdout(t *testing.T, runCommand func() int) (string, int) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	code := runCommand()
	os.Stdout = original
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return string(output), code
}

type installFailurePlatform struct {
	platform.Platform
}

func (installFailurePlatform) InstallService(context.Context, platform.ServiceOptions) error {
	return errors.New("service registration failed")
}

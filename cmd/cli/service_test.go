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
	paths.Socket = "/tmp/ghostgc-service-test.sock"
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
	wantArgs := []string{"daemon", "--service-log", "--config", paths.Config}
	if !reflect.DeepEqual(opts.Arguments, wantArgs) {
		t.Errorf("arguments = %#v, want %#v", opts.Arguments, wantArgs)
	}
	if _, err := os.Stat(paths.Config); !os.IsNotExist(err) {
		t.Errorf("service install created an unnecessary config: %v", err)
	}
	if _, err := os.Lstat(legacy); !os.IsNotExist(err) {
		t.Errorf("legacy executable still exists after migration: %v", err)
	}
}

func TestInstallBackgroundPersistsReconcileMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, link := executableFixture(t)
	stubExecutablePath(t, link)
	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	paths.Socket = "/tmp/ghostgc-reconcile-test.sock"
	fake := platformtest.New(501)
	mode := config.StartupReconcile
	if err := installBackground(context.Background(), &env{paths: paths}, fake, &mode); err != nil {
		t.Fatal(err)
	}
	want := []string{"daemon", "--service-log", "--mode", "reconcile", "--config", paths.Config}
	if got := fake.InstalledService().Arguments; !reflect.DeepEqual(got, want) {
		t.Fatalf("service arguments = %#v, want %#v", got, want)
	}
	if _, err := os.Stat(paths.Config); !os.IsNotExist(err) {
		t.Errorf("start created an unnecessary config: %v", err)
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

func TestStopUsesServiceUninstallPath(t *testing.T) {
	fake := platformtest.New(501)
	recorder := &uninstallRecordingPlatform{Platform: fake}
	if err := fake.InstallService(context.Background(), platform.ServiceOptions{Label: config.ServiceLabel}); err != nil {
		t.Fatal(err)
	}
	if err := uninstallBackground(context.Background(), recorder, nil); err != nil {
		t.Fatal(err)
	}
	if recorder.label != config.ServiceLabel {
		t.Fatalf("uninstalled label = %q, want %q", recorder.label, config.ServiceLabel)
	}
	state, err := fake.ServiceStatus(context.Background(), config.ServiceLabel)
	if err != nil {
		t.Fatal(err)
	}
	if state.Installed {
		t.Fatal("stop left the background service installed")
	}
	if err := uninstallBackground(context.Background(), fake, []string{"unexpected"}); err == nil {
		t.Fatal("stop accepted an unexpected argument")
	}
}

type uninstallRecordingPlatform struct {
	platform.Platform
	label string
}

func (p *uninstallRecordingPlatform) UninstallService(ctx context.Context, label string) error {
	p.label = label
	return p.Platform.UninstallService(ctx, label)
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

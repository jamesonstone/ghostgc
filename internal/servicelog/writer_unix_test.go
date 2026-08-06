//go:build darwin || linux

package servicelog

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaxBytesIsTenMegabytes(t *testing.T) {
	if MaxBytes != 10_000_000 {
		t.Fatalf("MaxBytes = %d, want 10,000,000", MaxBytes)
	}
}

func TestOpenBoundsExistingOutputAndEmptiesErrorLog(t *testing.T) {
	dir := privateTempDir(t)
	outPath := filepath.Join(dir, outputName)
	errPath := filepath.Join(dir, errorName)
	old := []byte("first record\nsecond record\nnewest record\n")
	if err := os.WriteFile(outPath, old, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(errPath, []byte("obsolete launchd error\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	w, err := openWithLimit(dir, 24)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	out := readFile(t, outPath)
	if len(out) > 18 {
		t.Fatalf("output size = %d, want <= 18-byte compaction watermark", len(out))
	}
	if !bytes.HasSuffix(out, []byte("newest record\n")) {
		t.Fatalf("output did not retain newest record: %q", out)
	}
	if got := len(readFile(t, errPath)); got != 0 {
		t.Fatalf("error log size = %d, want 0", got)
	}
	if got := len(out) + len(readFile(t, errPath)); got > 24 {
		t.Fatalf("combined log size = %d, want <= 24", got)
	}
}

func TestWriterCompactsBeforeAppend(t *testing.T) {
	dir := privateTempDir(t)
	w, err := openWithLimit(dir, 48)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })

	for _, record := range []string{
		"record-one-aaaaaaaa\n",
		"record-two-bbbbbbbb\n",
		"record-three-cccccc\n",
	} {
		if _, err := w.Write([]byte(record)); err != nil {
			t.Fatal(err)
		}
	}

	out := readFile(t, filepath.Join(dir, outputName))
	if len(out) > 48 {
		t.Fatalf("output size = %d, want <= 48", len(out))
	}
	if !strings.HasSuffix(string(out), "record-three-cccccc\n") {
		t.Fatalf("output did not retain newest record: %q", out)
	}
}

func TestWriterLeavesHeadroomAfterCompaction(t *testing.T) {
	dir := privateTempDir(t)
	w, err := openWithLimit(dir, 100)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })

	if _, err := w.Write(bytes.Repeat([]byte("a"), 100)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("newest\n")); err != nil {
		t.Fatal(err)
	}
	if got := len(readFile(t, filepath.Join(dir, outputName))); got > 83 {
		t.Fatalf("compacted size = %d, want <= 75-byte watermark plus append", got)
	}
}

func TestWriterBoundsSingleOversizedRecord(t *testing.T) {
	dir := privateTempDir(t)
	w, err := openWithLimit(dir, 16)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })

	record := []byte("discard-this-retain-this")
	n, err := w.Write(record)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(record) {
		t.Fatalf("Write count = %d, want %d", n, len(record))
	}
	if got, want := string(readFile(t, filepath.Join(dir, outputName))), string(record[len(record)-16:]); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestWriterSerializesConcurrentWritesWithinBound(t *testing.T) {
	dir := privateTempDir(t)
	w, err := openWithLimit(dir, 512)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })

	done := make(chan error, 8)
	for i := 0; i < cap(done); i++ {
		go func() {
			for j := 0; j < 100; j++ {
				if _, err := w.Write([]byte("one complete concurrent record\n")); err != nil {
					done <- err
					return
				}
			}
			done <- nil
		}()
	}
	for i := 0; i < cap(done); i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if got := len(readFile(t, filepath.Join(dir, outputName))); got > 512 {
		t.Fatalf("output size = %d, want <= 512", got)
	}
}

func TestOpenRefusesLinkedManagedPathsWithoutTouchingTarget(t *testing.T) {
	for _, name := range []string{outputName, errorName} {
		t.Run(name, func(t *testing.T) {
			dir := privateTempDir(t)
			target := filepath.Join(t.TempDir(), "important")
			want := []byte("must remain unchanged")
			if err := os.WriteFile(target, want, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}

			if _, err := openWithLimit(dir, 32); err == nil {
				t.Fatal("Open succeeded with a symlinked managed path")
			}
			if got := readFile(t, target); !bytes.Equal(got, want) {
				t.Fatalf("target changed to %q", got)
			}
		})
	}
}

func TestOpenRefusesHardLinkedAndWritableManagedFiles(t *testing.T) {
	t.Run("hard link", func(t *testing.T) {
		dir := privateTempDir(t)
		target := filepath.Join(t.TempDir(), "important")
		if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(target, filepath.Join(dir, outputName)); err != nil {
			t.Fatal(err)
		}
		if _, err := openWithLimit(dir, 32); err == nil {
			t.Fatal("Open succeeded with a hard-linked output")
		}
	})

	t.Run("group writable", func(t *testing.T) {
		dir := privateTempDir(t)
		path := filepath.Join(dir, outputName)
		if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o620); err != nil {
			t.Fatal(err)
		}
		if _, err := openWithLimit(dir, 32); err == nil {
			t.Fatal("Open succeeded with a group-writable output")
		}
	})
}

func TestWriterRefusesOutputMovedAfterOpen(t *testing.T) {
	dir := privateTempDir(t)
	w, err := openWithLimit(dir, 32)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })

	moved := filepath.Join(dir, "important")
	if err := os.Rename(filepath.Join(dir, outputName), moved); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("must not be written")); err == nil {
		t.Fatal("Write succeeded after the managed output moved")
	}
	if got := len(readFile(t, moved)); got != 0 {
		t.Fatalf("moved file size = %d, want 0", got)
	}
}

func TestWriterRefusesOutputLinkedAfterOpen(t *testing.T) {
	dir := privateTempDir(t)
	w, err := openWithLimit(dir, 32)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })

	outPath := filepath.Join(dir, outputName)
	if err := os.Link(outPath, filepath.Join(dir, "unexpected-link")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("must not be written")); err == nil {
		t.Fatal("Write succeeded after the managed output gained a hard link")
	}
	if got := len(readFile(t, outPath)); got != 0 {
		t.Fatalf("linked output size = %d, want 0", got)
	}
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return physical
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

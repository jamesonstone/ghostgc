//go:build darwin || linux

// Package servicelog owns the bounded diagnostic log used by the background
// service. It deliberately has no directory traversal or general cleanup API.
package servicelog

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

const (
	// MaxBytes is the hard bound for current background-service log files.
	MaxBytes   = 10_000_000
	outputName = "ghostgc.out.log"
	errorName  = "ghostgc.err.log"
)

// Writer retains the newest diagnostic output in one bounded regular file.
type Writer struct {
	mu     sync.Mutex
	file   *os.File
	path   string
	device uint64
	inode  uint64
	limit  int64
}

// Open validates the exact managed paths and opens the bounded output writer.
func Open(logDir string) (*Writer, error) {
	return openWithLimit(logDir, MaxBytes)
}

func openWithLimit(logDir string, limit int64) (*Writer, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("service log: invalid size limit %d", limit)
	}
	dir, err := validateDirectory(logDir)
	if err != nil {
		return nil, err
	}
	outPath := filepath.Join(dir, outputName)
	errPath := filepath.Join(dir, errorName)
	if err := inspectManagedPath(outPath); err != nil {
		return nil, err
	}
	if err := inspectManagedPath(errPath); err != nil {
		return nil, err
	}

	errFile, err := openExistingManagedFile(errPath)
	if err != nil {
		return nil, err
	}
	if errFile != nil {
		defer func() { _ = errFile.Close() }()
	}
	var errDevice, errInode uint64
	if errFile != nil {
		errDevice, errInode, err = fileIdentity(errFile)
		if err != nil {
			return nil, err
		}
	}
	file, err := openManagedFile(outPath, unix.O_CREAT|unix.O_RDWR|unix.O_APPEND)
	if err != nil {
		return nil, err
	}
	device, inode, err := fileIdentity(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	w := &Writer{file: file, path: outPath, device: device, inode: inode, limit: limit}
	if err := w.enforceBound(nil); err != nil {
		_ = file.Close()
		return nil, err
	}
	if errFile != nil {
		err = validateOpenPath(errFile, errPath, errDevice, errInode)
		if err == nil {
			err = errFile.Truncate(0)
		}
	}
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("service log: empty superseded error log: %w", err)
	}
	return w, nil
}

// Write appends p and reports it consumed once its newest bounded suffix is
// present in the managed file. A single oversized record retains its suffix.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if err := w.enforceBound(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close closes the managed output file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *Writer) enforceBound(p []byte) error {
	if err := w.validateCurrentPath(); err != nil {
		return err
	}
	info, err := w.file.Stat()
	if err != nil {
		return fmt.Errorf("service log: inspect output: %w", err)
	}
	if info.Size() <= w.limit && int64(len(p)) <= w.limit-info.Size() {
		return writeAll(w.file, p)
	}

	if int64(len(p)) >= w.limit {
		p = p[len(p)-int(w.limit):]
		if err := w.file.Truncate(0); err != nil {
			return fmt.Errorf("service log: compact output: %w", err)
		}
		return writeAll(w.file, p)
	}

	keep := w.limit * 3 / 4
	if available := w.limit - int64(len(p)); keep > available {
		keep = available
	}
	tail, err := readTail(w.file, info.Size(), keep)
	if err != nil {
		return err
	}
	if i := bytes.IndexByte(tail, '\n'); i >= 0 {
		tail = tail[i+1:]
	}
	if err := w.file.Truncate(0); err != nil {
		return fmt.Errorf("service log: compact output: %w", err)
	}
	if err := writeAll(w.file, tail); err != nil {
		return err
	}
	return writeAll(w.file, p)
}

func readTail(file *os.File, size, keep int64) ([]byte, error) {
	if keep <= 0 || size <= 0 {
		return nil, nil
	}
	if keep > size {
		keep = size
	}
	tail := make([]byte, int(keep))
	n, err := file.ReadAt(tail, size-keep)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("service log: read output tail: %w", err)
	}
	return tail[:n], nil
}

func writeAll(file *os.File, p []byte) error {
	for len(p) > 0 {
		n, err := file.Write(p)
		if err != nil {
			return fmt.Errorf("service log: write output: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("service log: write output: %w", io.ErrShortWrite)
		}
		p = p[n:]
	}
	return nil
}

func (w *Writer) validateCurrentPath() error {
	return validateOpenPath(w.file, w.path, w.device, w.inode)
}

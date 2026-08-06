package codex

import (
	"os"
	"path/filepath"
	"syscall"
)

func secureDirectory(path string, uid uint32) (string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
		return "", false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uid {
		return "", false
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil || canonical != filepath.Clean(abs) {
		return "", false
	}
	return canonical, true
}

func securePath(path, root string, uid uint32) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) {
		return false
	}
	current := root
	for _, part := range splitPath(rel) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
			return false
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uid {
			return false
		}
		if current == path && (!info.Mode().IsRegular() || stat.Nlink != 1) {
			return false
		}
	}
	canonical, err := filepath.EvalSymlinks(path)
	return err == nil && canonical == filepath.Clean(path) && within(root, canonical)
}

func splitPath(path string) []string {
	var out []string
	for path != "." && path != "" {
		dir, base := filepath.Split(path)
		out = append([]string{base}, out...)
		path = filepath.Clean(dir)
	}
	return out
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && rel != "" && rel[0] != filepath.Separator
}

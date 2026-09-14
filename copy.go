package svcforge

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func copyFileAtomic(source, target string, mode os.FileMode) error {
	sourceClean, err := filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("resolve source path: %w", err)
	}
	targetClean, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve target path: %w", err)
	}
	if sourceClean == targetClean {
		return os.Chmod(targetClean, mode.Perm())
	}
	input, err := os.Open(sourceClean)
	if err != nil {
		return fmt.Errorf("open source binary %s: %w", sourceClean, err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return fmt.Errorf("stat source binary %s: %w", sourceClean, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source binary %s is not a regular file", sourceClean)
	}
	dir := filepath.Dir(targetClean)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create binary directory %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".svcforge-binary-*")
	if err != nil {
		return fmt.Errorf("create staged binary: %w", err)
	}
	tmpPath := tmp.Name()
	removeTemp := true
	defer func() {
		_ = tmp.Close()
		if removeTemp {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(mode.Perm()); err != nil {
		return fmt.Errorf("chmod staged binary: %w", err)
	}
	if _, err := io.Copy(tmp, input); err != nil {
		return fmt.Errorf("copy staged binary: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync staged binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close staged binary: %w", err)
	}
	if err := os.Rename(tmpPath, targetClean); err != nil {
		return fmt.Errorf("publish binary %s: %w", targetClean, err)
	}
	removeTemp = false
	return nil
}

func copyPath(source, target string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	switch {
	case info.Mode().IsRegular():
		return copyRegularFile(source, target, info)
	case info.IsDir():
		if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
			return err
		}
		if err := preserveOwnership(target, info, false); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyPath(filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name())); err != nil {
				return err
			}
		}
		if err := os.Chmod(target, info.Mode().Perm()); err != nil {
			return err
		}
		return os.Chtimes(target, info.ModTime(), info.ModTime())
	case info.Mode()&os.ModeSymlink != 0:
		link, err := os.Readlink(source)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.Symlink(link, target); err != nil {
			return err
		}
		return preserveOwnership(target, info, true)
	default:
		return fmt.Errorf("cannot back up special file %s (%s)", source, info.Mode().Type())
	}
}

func copyRegularFile(source, target string, info os.FileInfo) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = output.Close()
		if !ok {
			_ = os.Remove(target)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := preserveOwnership(target, info, false); err != nil {
		return err
	}
	if err := os.Chmod(target, info.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Chtimes(target, info.ModTime(), info.ModTime()); err != nil {
		return err
	}
	ok = true
	return nil
}

func restorePath(backupPath, target string, existed bool) error {
	if err := os.RemoveAll(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !existed {
		return nil
	}
	return copyPath(backupPath, target)
}

func pathContains(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	rel, err := filepath.Rel(parent, child)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

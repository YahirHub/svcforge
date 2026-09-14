package svcforge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type snapshotSource struct {
	Path     string
	Optional bool
}

type backupEntry struct {
	Source   string `json:"source"`
	Relative string `json:"relative"`
	Existed  bool   `json:"existed"`
}

type backupSnapshot struct {
	Path      string        `json:"-"`
	CreatedAt time.Time     `json:"created_at"`
	Entries   []backupEntry `json:"entries"`
}

func createBackupSnapshot(root string, now time.Time, sources []snapshotSource) (*backupSnapshot, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create backup root: %w", err)
	}
	prefix := now.UTC().Format("20060102T150405.000000000Z") + "-"
	dir, err := os.MkdirTemp(root, prefix)
	if err != nil {
		return nil, fmt.Errorf("create backup snapshot: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("chmod backup snapshot: %w", err)
	}
	snapshot := &backupSnapshot{Path: dir, CreatedAt: now.UTC()}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(dir)
		}
	}()
	for index, source := range sources {
		cleanSource := filepath.Clean(source.Path)
		entry := backupEntry{
			Source:   cleanSource,
			Relative: filepath.Join("entries", fmt.Sprintf("%03d-%s", index, safeBackupName(filepath.Base(cleanSource)))),
		}
		_, statErr := os.Lstat(cleanSource)
		switch {
		case statErr == nil:
			entry.Existed = true
			if err := copyPath(cleanSource, filepath.Join(dir, entry.Relative)); err != nil {
				return nil, fmt.Errorf("back up %s: %w", cleanSource, err)
			}
		case errors.Is(statErr, os.ErrNotExist) && source.Optional:
			entry.Existed = false
		case errors.Is(statErr, os.ErrNotExist):
			return nil, fmt.Errorf("required backup target %s does not exist", cleanSource)
		default:
			return nil, fmt.Errorf("stat backup source %s: %w", cleanSource, statErr)
		}
		snapshot.Entries = append(snapshot.Entries, entry)
	}
	metadata, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return nil, err
	}
	metadata = append(metadata, '\n')
	if err := writeFileAtomic(filepath.Join(dir, "snapshot.json"), metadata, 0o600); err != nil {
		return nil, err
	}
	ok = true
	return snapshot, nil
}

func (snapshot *backupSnapshot) Restore() error {
	if snapshot == nil {
		return nil
	}
	var failures []string
	for i := len(snapshot.Entries) - 1; i >= 0; i-- {
		entry := snapshot.Entries[i]
		if err := restorePath(filepath.Join(snapshot.Path, entry.Relative), entry.Source, entry.Existed); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", entry.Source, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("restore backup snapshot: %s", strings.Join(failures, "; "))
	}
	return nil
}

func pruneBackups(root string, keep int) error {
	if keep == 0 {
		keep = 3
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	type candidate struct {
		name string
		info os.FileInfo
	}
	var dirs []candidate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		dirs = append(dirs, candidate{name: entry.Name(), info: info})
	}
	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i].info.ModTime().Equal(dirs[j].info.ModTime()) {
			return dirs[i].name > dirs[j].name
		}
		return dirs[i].info.ModTime().After(dirs[j].info.ModTime())
	})
	if keep >= len(dirs) {
		return nil
	}
	for _, item := range dirs[keep:] {
		if err := os.RemoveAll(filepath.Join(root, item.name)); err != nil {
			return err
		}
	}
	return nil
}

func safeBackupName(value string) string {
	if value == "" || value == "." || value == string(filepath.Separator) {
		return "root"
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "entry"
	}
	return b.String()
}

// Copyright (C) 2026 Yota Hamada
// SPDX-License-Identifier: GPL-3.0-or-later

package workbook

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
)

type workbookPathLock struct {
	token chan struct{}
	refs  int
}

var workbookPathLocks = struct {
	sync.Mutex
	entries map[string]*workbookPathLock
}{
	entries: make(map[string]*workbookPathLock),
}

func acquireWorkbookPathLock(ctx context.Context, path string) (func(), error) {
	return acquireWorkbookPathLockKey(ctx, workbookPathLockKey(path))
}

func acquireWorkbookPathLocks(ctx context.Context, paths ...string) (func(), error) {
	keys := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		keys[workbookPathLockKey(path)] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)

	releases := make([]func(), 0, len(ordered))
	for _, key := range ordered {
		release, err := acquireWorkbookPathLockKey(ctx, key)
		if err != nil {
			for _, release := range slices.Backward(releases) {
				release()
			}
			return nil, err
		}
		releases = append(releases, release)
	}
	return func() {
		for _, release := range slices.Backward(releases) {
			release()
		}
	}, nil
}

func acquireWorkbookPathLockKey(ctx context.Context, key string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workbookPathLocks.Lock()
	entry := workbookPathLocks.entries[key]
	if entry == nil {
		entry = &workbookPathLock{token: make(chan struct{}, 1)}
		entry.token <- struct{}{}
		workbookPathLocks.entries[key] = entry
	}
	entry.refs++
	workbookPathLocks.Unlock()

	select {
	case <-ctx.Done():
		releaseWorkbookPathLock(key, entry, false)
		return nil, ctx.Err()
	case <-entry.token:
		return func() { releaseWorkbookPathLock(key, entry, true) }, nil
	}
}

func releaseWorkbookPathLock(key string, entry *workbookPathLock, held bool) {
	if held {
		entry.token <- struct{}{}
	}
	workbookPathLocks.Lock()
	entry.refs--
	if entry.refs == 0 && workbookPathLocks.entries[key] == entry {
		delete(workbookPathLocks.entries, key)
	}
	workbookPathLocks.Unlock()
}

func workbookPathLockKey(path string) string {
	path = absoluteCleanPath(path)
	if resolved, err := resolveExistingAncestor(path); err == nil {
		path = resolved
	}
	if filesystemIsCaseInsensitive(path) {
		path = strings.ToLower(path)
	}
	return path
}

func resolveExistingAncestor(path string) (string, error) {
	suffix := make([]string, 0)
	current := path
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for _, part := range slices.Backward(suffix) {
				resolved = filepath.Join(resolved, part)
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func filesystemIsCaseInsensitive(path string) bool {
	dir := filepath.Dir(path)
	for {
		info, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) {
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
			continue
		}
		if err == nil {
			name := filepath.Base(dir)
			if alternate, ok := alternateASCIICase(name); ok {
				alternateInfo, alternateErr := os.Lstat(filepath.Join(filepath.Dir(dir), alternate))
				switch {
				case alternateErr == nil:
					return os.SameFile(info, alternateInfo)
				case errors.Is(alternateErr, os.ErrNotExist):
					return false
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return runtime.GOOS == "windows" || runtime.GOOS == "darwin"
}

func alternateASCIICase(value string) (string, bool) {
	bytes := []byte(value)
	for idx, ch := range bytes {
		switch {
		case ch >= 'a' && ch <= 'z':
			bytes[idx] = ch - ('a' - 'A')
			return string(bytes), true
		case ch >= 'A' && ch <= 'Z':
			bytes[idx] = ch + ('a' - 'A')
			return string(bytes), true
		}
	}
	return "", false
}

func absoluteCleanPath(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

// Copyright (C) 2026 Yota Hamada
// SPDX-License-Identifier: GPL-3.0-or-later

package workbook

import (
	"context"
	"path/filepath"
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := workbookPathLockKey(path)
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
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return absoluteCleanPath(resolved)
	}
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		return filepath.Join(resolved, filepath.Base(path))
	}
	return path
}

func absoluteCleanPath(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package runners

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	ddlog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const taskLedgerPruneInterval = 10 * time.Minute

var errTaskAlreadyAccepted = errors.New("task was already accepted for execution")

// taskLedger records the IDs of accepted tasks until they expire, so each task
// runs at most once. When dir is set, entries are also kept as marker files
// named after the task ID hash, with the task expiration as their mtime, so they
// outlive the process (the split-mode executor exits when idle).
type taskLedger struct {
	dir string
	now func() time.Time

	mu        sync.Mutex
	entries   map[string]time.Time
	lastPrune time.Time
}

// newTaskLedger returns a ledger persisted under dir, or an in-memory one when
// dir is empty or not writable. now defaults to time.Now when nil.
func newTaskLedger(dir string, now func() time.Time) *taskLedger {
	if dir != "" {
		if err := checkLedgerDir(dir); err != nil {
			ddlog.Warnf("Accepted task IDs will not persist across restarts: %v", err)
			dir = ""
		}
	}
	if now == nil {
		now = time.Now
	}
	return &taskLedger{dir: dir, now: now}
}

func checkLedgerDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	probe, err := os.CreateTemp(dir, ".probe-")
	if err != nil {
		return err
	}
	probe.Close()
	return os.Remove(probe.Name())
}

// accept records taskID until expiresAt. It fails if taskID is already recorded
// or cannot be recorded.
func (l *taskLedger) accept(taskID string, expiresAt time.Time) error {
	if taskID == "" {
		return util.NewPARError(aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, errors.New("task id is missing"))
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if now.Sub(l.lastPrune) >= taskLedgerPruneInterval {
		l.prune(now)
		l.lastPrune = now
	}

	if _, ok := l.entries[taskID]; ok {
		return util.NewPARError(aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, errTaskAlreadyAccepted)
	}
	if l.dir != "" {
		if err := l.writeMarker(taskID, expiresAt); err != nil {
			if errors.Is(err, fs.ErrExist) {
				err = errTaskAlreadyAccepted
			}
			return util.NewPARError(aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, err)
		}
	}
	if l.entries == nil {
		l.entries = make(map[string]time.Time)
	}
	l.entries[taskID] = expiresAt
	return nil
}

// writeMarker creates the marker exclusively, so concurrent processes sharing
// dir cannot both accept the same task. It hard-links a file already carrying
// the expiration, which a concurrent prune would otherwise see as expired.
func (l *taskLedger) writeMarker(taskID string, expiresAt time.Time) error {
	path := filepath.Join(l.dir, markerName(taskID))
	f, err := os.CreateTemp(l.dir, ".accept-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = os.Chtimes(f.Name(), expiresAt, expiresAt)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Link(f.Name(), path)
	}
	if err != nil {
		return err
	}
	if err := syncDir(l.dir); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

func (l *taskLedger) prune(now time.Time) {
	for id, expiresAt := range l.entries {
		if !now.Before(expiresAt) {
			delete(l.entries, id)
		}
	}
	if l.dir == "" {
		return
	}
	dirEntries, err := os.ReadDir(l.dir)
	if err != nil {
		ddlog.Warnf("Could not list accepted task IDs: %v", err)
		return
	}
	for _, entry := range dirEntries {
		if !isMarkerName(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if !now.Before(info.ModTime()) {
			os.Remove(filepath.Join(l.dir, entry.Name()))
		}
	}
}

func markerName(taskID string) string {
	sum := sha256.Sum256([]byte(taskID))
	return hex.EncodeToString(sum[:])
}

func isMarkerName(name string) bool {
	if len(name) != 2*sha256.Size {
		return false
	}
	_, err := hex.DecodeString(name)
	return err == nil
}

func syncDir(dir string) error {
	// Windows cannot open directories for syncing.
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	// AIX cannot fsync directories (EINVAL/EBADF).
	if err := d.Sync(); err != nil && (runtime.GOOS != "aix" || (!errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.EBADF))) {
		return err
	}
	return nil
}

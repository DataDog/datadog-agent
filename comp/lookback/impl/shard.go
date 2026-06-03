// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	walExtension = ".wal"
)

// walFile represents a sealed WAL file on disk.
type walFile struct {
	path        string
	windowStart int64 // Unix second encoded in filename
	size        int64
}

// shard manages one WAL shard: a directory containing timestamped .wal files
// and a write buffer. Context metadata is owned by the shared contextFile.
type shard struct {
	mu          sync.Mutex
	dir         string
	buf         []byte   // in-memory write buffer
	maxBufSize  int
	activeF     *os.File // currently open WAL file (append-only)
	windowStart int64    // Unix second: start of the active window
}

func newShard(dir string, windowStartSec int64, maxBufSize int) (*shard, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("lookback shard %s: mkdir: %w", dir, err)
	}
	s := &shard{
		dir:         dir,
		maxBufSize:  maxBufSize,
		windowStart: windowStartSec,
	}
	if err := s.openActiveFile(); err != nil {
		return nil, err
	}
	return s, nil
}

// openActiveFile opens (or creates) the WAL file for the current window.
func (s *shard) openActiveFile() error {
	name := fmt.Sprintf("%d%s", s.windowStart, walExtension)
	path := filepath.Join(s.dir, name)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("lookback WAL open %s: %w", path, err)
	}
	s.activeF = f
	return nil
}

// writeRecord appends a record to the in-memory buffer, flushing to disk
// automatically when the buffer is full. Must be called with s.mu held.
func (s *shard) writeRecord(r record) error {
	s.buf = appendRecord(s.buf, r)
	if len(s.buf) >= s.maxBufSize {
		return s.flushLocked()
	}
	return nil
}

// flushLocked writes the in-memory buffer to the active file.
// Must be called with s.mu held.
func (s *shard) flushLocked() error {
	if len(s.buf) == 0 {
		return nil
	}
	if _, err := s.activeF.Write(s.buf); err != nil {
		return fmt.Errorf("lookback WAL write: %w", err)
	}
	s.buf = s.buf[:0]
	return nil
}

// rotate seals the active file and starts a new one for newWindowStartSec.
// The entire sequence is atomic under s.mu.
func (s *shard) rotate(newWindowStartSec int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.flushLocked(); err != nil {
		return err
	}
	if err := s.activeF.Sync(); err != nil {
		return fmt.Errorf("lookback WAL sync: %w", err)
	}
	if err := s.activeF.Close(); err != nil {
		return fmt.Errorf("lookback WAL close: %w", err)
	}
	s.activeF = nil

	s.windowStart = newWindowStartSec
	return s.openActiveFile()
}

// stop flushes and syncs the active file, then closes it.
func (s *shard) stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.flushLocked(); err != nil {
		return err
	}
	if s.activeF != nil {
		if err := s.activeF.Sync(); err != nil {
			return err
		}
		if err := s.activeF.Close(); err != nil {
			return err
		}
		s.activeF = nil
	}
	return nil
}

// sealedFiles returns the sealed WAL files (all but the active window) sorted
// by windowStart ascending. Does not acquire s.mu (reads filesystem only).
func (s *shard) sealedFiles() ([]walFile, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var files []walFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), walExtension) {
			continue
		}
		ws, err := parseWALFilename(e.Name())
		if err != nil {
			continue
		}
		s.mu.Lock()
		active := s.windowStart
		s.mu.Unlock()
		if ws == active {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, walFile{
			path:        filepath.Join(s.dir, e.Name()),
			windowStart: ws,
			size:        info.Size(),
		})
	}
	for i := 1; i < len(files); i++ {
		for j := i; j > 0 && files[j].windowStart < files[j-1].windowStart; j-- {
			files[j], files[j-1] = files[j-1], files[j]
		}
	}
	return files, nil
}

// readRecordsFromFile reads all records from a single WAL file path.
func readRecordsFromFile(path string) ([]record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readAllRecords(f)
}

// deleteFile removes a WAL file from disk.
func (s *shard) deleteFile(path string) error {
	return os.Remove(path)
}

// parseWALFilename extracts the window-start Unix second from "<unix>.wal".
func parseWALFilename(name string) (int64, error) {
	base := strings.TrimSuffix(name, walExtension)
	v, err := strconv.ParseInt(base, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("lookback: invalid WAL filename %q: %w", name, err)
	}
	return v, nil
}

// filesInRange returns sealed WAL files whose window overlaps [startNs, stopNs).
func filesInRange(files []walFile, startUs, stopUs int64, rotationIntervalSec int64) []walFile {
	var out []walFile
	for _, f := range files {
		windowEndUs := (f.windowStart + rotationIntervalSec) * 1_000_000
		windowStartUs := f.windowStart * 1_000_000
		if windowStartUs < stopUs && windowEndUs > startUs {
			out = append(out, f)
		}
	}
	return out
}

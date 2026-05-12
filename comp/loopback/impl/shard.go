// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loopbackimpl

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	walExtension     = ".wal"
	catalogFilename  = "catalog.cat"
	catalogTmpSuffix = ".tmp"
)

// walFile represents a sealed WAL file on disk.
type walFile struct {
	path        string
	windowStart int64 // Unix second encoded in filename
	size        int64
}

// shard manages one WAL shard: a directory containing timestamped .wal files,
// a write buffer, and a context catalog file.
type shard struct {
	mu          sync.Mutex
	dir         string
	buf         []byte   // in-memory write buffer
	maxBufSize  int
	activeF     *os.File // currently open WAL file (append-only)
	windowStart int64    // Unix second: start of the active window
	catalogF    *os.File // append-only context catalog
	reg         *contextRegistry
}

func newShard(dir string, windowStartSec int64, maxBufSize int, reg *contextRegistry) (*shard, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("loopback shard %s: mkdir: %w", dir, err)
	}
	s := &shard{
		dir:         dir,
		maxBufSize:  maxBufSize,
		windowStart: windowStartSec,
		reg:         reg,
	}
	if err := s.loadCatalog(); err != nil {
		return nil, err
	}
	if err := s.openActiveFile(); err != nil {
		return nil, err
	}
	return s, nil
}

// loadCatalog reads the catalog file to restore the in-memory registry.
func (s *shard) loadCatalog() error {
	path := filepath.Join(s.dir, catalogFilename)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("loopback catalog open %s: %w", path, err)
	}
	s.catalogF = f

	// Read existing entries and populate registry.
	s.reg.mu.Lock()
	defer s.reg.mu.Unlock()
	if err := readCatalog(f, s.reg); err != nil {
		return fmt.Errorf("loopback catalog read %s: %w", path, err)
	}
	return nil
}

// openActiveFile opens (or creates) the WAL file for the current window.
func (s *shard) openActiveFile() error {
	name := fmt.Sprintf("%d%s", s.windowStart, walExtension)
	path := filepath.Join(s.dir, name)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("loopback WAL open %s: %w", path, err)
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
		return fmt.Errorf("loopback WAL write: %w", err)
	}
	s.buf = s.buf[:0]
	return nil
}

// maybeRegisterKey checks if the context key is new and appends it to the
// catalog if so. Must be called with s.mu held.
func (s *shard) maybeRegisterKey(key uint64, name string, tags []string) error {
	_, _, exists := s.reg.getEntry(key)
	if exists {
		return nil
	}
	// Register in memory.
	s.reg.registerWithKey(key, name, tags)
	// Append to catalog file.
	return appendCatalogEntry(s.catalogF, key, name, tags)
}

// rotate seals the active file and starts a new one for newWindowStartSec.
// The entire sequence is atomic under s.mu: no record can land on a closed file.
func (s *shard) rotate(newWindowStartSec int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Flush remaining buffer.
	if err := s.flushLocked(); err != nil {
		return err
	}
	// 2. fdatasync.
	if err := s.activeF.Sync(); err != nil {
		return fmt.Errorf("loopback WAL sync: %w", err)
	}
	// 3. Close.
	if err := s.activeF.Close(); err != nil {
		return fmt.Errorf("loopback WAL close: %w", err)
	}
	s.activeF = nil

	// 4. Compact catalog (atomic rewrite).
	if err := s.compactCatalogLocked(); err != nil {
		// Non-fatal: log but proceed.
		_ = err
	}

	// 5. Open new file.
	s.windowStart = newWindowStartSec
	if err := s.openActiveFile(); err != nil {
		return err
	}
	return nil
}

// compactCatalogLocked rewrites the catalog file with only currently-known
// context keys, preventing unbounded growth. Must be called with s.mu held.
func (s *shard) compactCatalogLocked() error {
	entries := s.reg.allEntries()
	tmpPath := filepath.Join(s.dir, catalogFilename+catalogTmpSuffix)

	tmp, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	for key, e := range entries {
		if err := appendCatalogEntry(tmp, key, e.name, e.tags); err != nil {
			tmp.Close()
			os.Remove(tmpPath)
			return err
		}
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	tmp.Close()

	catPath := filepath.Join(s.dir, catalogFilename)
	if err := os.Rename(tmpPath, catPath); err != nil {
		os.Remove(tmpPath)
		return err
	}
	// Reopen the catalog for further appends.
	if s.catalogF != nil {
		s.catalogF.Close()
	}
	f, err := os.OpenFile(catPath, os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	s.catalogF = f
	return nil
}

// stop flushes and syncs the active file, then closes all file handles.
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
	if s.catalogF != nil {
		_ = s.catalogF.Close()
		s.catalogF = nil
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
			continue // skip active window
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
	// Sort ascending by windowStart.
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
		return 0, fmt.Errorf("loopback: invalid WAL filename %q: %w", name, err)
	}
	return v, nil
}

// --- Catalog binary format ---
//
// Each entry:
//   key       uint64  (8 bytes)
//   nameLen   uint16  (2 bytes)
//   name      []byte  (nameLen bytes)
//   tagsCount uint16  (2 bytes)
//   for each tag:
//     tagLen  uint16  (2 bytes)
//     tag     []byte  (tagLen bytes)

func appendCatalogEntry(w io.Writer, key uint64, name string, tags []string) error {
	var hdr [8 + 2]byte
	binary.BigEndian.PutUint64(hdr[0:8], key)
	binary.BigEndian.PutUint16(hdr[8:10], uint16(len(name)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if _, err := io.WriteString(w, name); err != nil {
		return err
	}
	var cnt [2]byte
	binary.BigEndian.PutUint16(cnt[:], uint16(len(tags)))
	if _, err := w.Write(cnt[:]); err != nil {
		return err
	}
	for _, tag := range tags {
		var tl [2]byte
		binary.BigEndian.PutUint16(tl[:], uint16(len(tag)))
		if _, err := w.Write(tl[:]); err != nil {
			return err
		}
		if _, err := io.WriteString(w, tag); err != nil {
			return err
		}
	}
	return nil
}

// readCatalog parses all catalog entries from r and registers them in reg.
// reg.mu must be held by the caller for writing.
func readCatalog(r io.Reader, reg *contextRegistry) error {
	for {
		var hdr [10]byte
		_, err := io.ReadFull(r, hdr[:])
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}
		if err != nil {
			return err
		}
		key := binary.BigEndian.Uint64(hdr[0:8])
		nameLen := binary.BigEndian.Uint16(hdr[8:10])

		nameBuf := make([]byte, nameLen)
		if _, err := io.ReadFull(r, nameBuf); err != nil {
			return err
		}
		var cntBuf [2]byte
		if _, err := io.ReadFull(r, cntBuf[:]); err != nil {
			return err
		}
		tagsCount := binary.BigEndian.Uint16(cntBuf[:])
		tags := make([]string, tagsCount)
		for i := range tags {
			var tl [2]byte
			if _, err := io.ReadFull(r, tl[:]); err != nil {
				return err
			}
			tagBuf := make([]byte, binary.BigEndian.Uint16(tl[:]))
			if _, err := io.ReadFull(r, tagBuf); err != nil {
				return err
			}
			tags[i] = string(tagBuf)
		}
		reg.registerEntryLocked(key, string(nameBuf), tags)
	}
}

// filesInRange returns sealed WAL files whose window overlaps [startNs, stopNs).
// windowDurationSec is the rotation interval in seconds.
func filesInRange(files []walFile, startNs, stopNs int64, windowDurationSec int64) []walFile {
	var out []walFile
	for _, f := range files {
		windowEndSec := f.windowStart + windowDurationSec
		windowEndNs := windowEndSec * int64(time.Second)
		windowStartNs := f.windowStart * int64(time.Second)
		if windowStartNs < stopNs && windowEndNs > startNs {
			out = append(out, f)
		}
	}
	return out
}

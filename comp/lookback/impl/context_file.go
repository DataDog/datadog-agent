// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/twmb/murmur3"
)

// contextEntry holds the canonical name and sorted tags for a context key.
type contextEntry struct {
	name string
	tags []string // sorted
}

// contextFile is the single shared contexts.bin file that maps context keys to
// their metric name and tags. It is the sole persistent context store; no
// in-memory name→key map is maintained.
//
// Write path (hot): a lock-free bloom filter gates appends so the mutex is
// almost never acquired. Read path (Flush, cold): a linear scan from the start.
type contextFile struct {
	mu    sync.Mutex
	f     *os.File
	bloom *contextSet
}

// newContextFile opens (or creates) the contexts.bin file at path.
// Existing entries are scanned once to populate the bloom filter, ensuring
// keys already on disk are never re-written.
func newContextFile(path string) (*contextFile, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("lookback context file open %s: %w", path, err)
	}
	cf := &contextFile{f: f, bloom: newContextSet(0)}
	if err := cf.loadIntoBloom(); err != nil {
		f.Close()
		return nil, fmt.Errorf("lookback context file load %s: %w", path, err)
	}
	return cf, nil
}

// loadIntoBloom reads all existing entries from the beginning of the file and
// marks each key in the bloom filter. Called once at startup.
func (cf *contextFile) loadIntoBloom() error {
	if _, err := cf.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	for {
		key, _, _, err := readContextEntry(cf.f)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}
		if err != nil {
			return err
		}
		cf.bloom.IsKnown(key) // marks bits as a side-effect on first call
	}
}

// maybeWrite appends a context entry to the file if the key has not been seen
// before. The bloom filter check is lock-free; the mutex is acquired only when
// a new entry must actually be written.
func (cf *contextFile) maybeWrite(key uint64, name string, tags []string) error {
	if cf.bloom.IsKnown(key) {
		return nil
	}
	cf.mu.Lock()
	defer cf.mu.Unlock()
	return appendContextEntry(cf.f, key, name, tags)
}

// scan reads all entries from the beginning of the file and returns those whose
// name matches and whose tags are a superset of the requested filter tags.
// If tags is nil, all entries matching the name are returned.
// The returned map is used as the resolve function for aggregateRecords.
func (cf *contextFile) scan(name string, filterTags []string) (map[uint64]contextEntry, error) {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if _, err := cf.f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("lookback context scan seek: %w", err)
	}
	result := make(map[uint64]contextEntry)
	for {
		key, entryName, entryTags, err := readContextEntry(cf.f)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return result, fmt.Errorf("lookback context scan read: %w", err)
		}
		if entryName != name {
			continue
		}
		if filterTags != nil && !tagsSubset(filterTags, entryTags) {
			continue
		}
		result[key] = contextEntry{name: entryName, tags: entryTags}
	}
	return result, nil
}

// close flushes and closes the file handle.
func (cf *contextFile) close() error {
	cf.mu.Lock()
	defer cf.mu.Unlock()
	if err := cf.f.Sync(); err != nil {
		return err
	}
	return cf.f.Close()
}

// --- Binary format ---
//
// Each entry:
//   key       uint64  (8 bytes)
//   nameLen   uint16  (2 bytes)
//   name      []byte  (nameLen bytes)
//   tagsCount uint16  (2 bytes)
//   for each tag:
//     tagLen  uint16  (2 bytes)
//     tag     []byte  (tagLen bytes)

func appendContextEntry(w io.Writer, key uint64, name string, tags []string) error {
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

func readContextEntry(r io.Reader) (key uint64, name string, tags []string, err error) {
	var hdr [10]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return
	}
	key = binary.BigEndian.Uint64(hdr[0:8])
	nameLen := binary.BigEndian.Uint16(hdr[8:10])

	nameBuf := make([]byte, nameLen)
	if _, err = io.ReadFull(r, nameBuf); err != nil {
		return
	}
	name = string(nameBuf)

	var cntBuf [2]byte
	if _, err = io.ReadFull(r, cntBuf[:]); err != nil {
		return
	}
	tagsCount := binary.BigEndian.Uint16(cntBuf[:])
	tags = make([]string, tagsCount)
	for i := range tags {
		var tl [2]byte
		if _, err = io.ReadFull(r, tl[:]); err != nil {
			return
		}
		tagBuf := make([]byte, binary.BigEndian.Uint16(tl[:]))
		if _, err = io.ReadFull(r, tagBuf); err != nil {
			return
		}
		tags[i] = string(tagBuf)
	}
	return
}

// --- Utility functions (moved from registry.go) ---

func syntheticKey(name string, sortedTags []string) uint64 {
	return murmur3.Sum64([]byte(name + "|" + strings.Join(sortedTags, ",")))
}

func sortedTagsCopy(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, len(tags))
	copy(out, tags)
	slices.Sort(out)
	return out
}

// tagsSubset reports whether every tag in requested is present in registered.
func tagsSubset(requested, registered []string) bool {
	for _, req := range requested {
		if !slices.Contains(registered, req) {
			return false
		}
	}
	return true
}

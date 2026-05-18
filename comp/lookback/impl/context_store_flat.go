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
	"path/filepath"
	"sync"

	"github.com/twmb/murmur3"
)

// flatContextStore is a contextStore backed by an append-only binary file.
// It provides O(1) writes and O(file_size) scans. Suitable as a fallback or
// for comparison testing; the default implementation is boltContextStore.
type flatContextStore struct {
	mu sync.Mutex
	f  *os.File
}

func newFlatContextStore(path string) (*flatContextStore, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("lookback flat store open %s: %w", path, err)
	}
	return &flatContextStore{f: f}, nil
}

func (s *flatContextStore) maybeWrite(key uint64, name string, tags []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return appendFlatEntry(s.f, key, name, tags)
}

func (s *flatContextStore) scan(name string, filterTags []string) (map[uint64]contextEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("lookback flat scan seek: %w", err)
	}
	result := make(map[uint64]contextEntry)
	for {
		key, entryName, entryTags, err := readFlatEntry(s.f)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return result, fmt.Errorf("lookback flat scan read: %w", err)
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

func (s *flatContextStore) loadKeys(fn func(uint64)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	for {
		key, _, _, err := readFlatEntry(s.f)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}
		if err != nil {
			return err
		}
		fn(key)
	}
}

func (s *flatContextStore) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.f.Sync(); err != nil {
		return err
	}
	return s.f.Close()
}

// --- Binary format ---
//
//	key       uint64  (8 bytes)
//	nameLen   uint16  (2 bytes)
//	name      []byte  (nameLen bytes)
//	tagsCount uint16  (2 bytes)
//	for each tag:
//	  tagLen  uint16  (2 bytes)
//	  tag     []byte  (tagLen bytes)
func appendFlatEntry(w io.Writer, key uint64, name string, tags []string) error {
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

func readFlatEntry(r io.Reader) (key uint64, name string, tags []string, err error) {
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

// shardedFlatContextStore is a contextStore backed by N independent append-only
// binary shard files, routed by murmur32(name) % N. Each shard has its own
// mutex, so writes to different metric names proceed concurrently.
//
// Compared to a single flatContextStore:
//   - maybeWrite: N-way parallelism, same O(1) append cost per shard
//   - scan(name): reads only 1/N of the total data — O(file_size/N)
//   - loadKeys: iterates all N shards sequentially at startup
//
// This is the default contextStore implementation.
type shardedFlatContextStore struct {
	shards []*flatContextStore
	n      uint32
}

func newShardedFlatContextStore(dir string, n int) (*shardedFlatContextStore, error) {
	shards := make([]*flatContextStore, n)
	for i := range n {
		s, err := newFlatContextStore(filepath.Join(dir, fmt.Sprintf("contexts-%02d.bin", i)))
		if err != nil {
			for j := range i {
				_ = shards[j].close()
			}
			return nil, err
		}
		shards[i] = s
	}
	return &shardedFlatContextStore{shards: shards, n: uint32(n)}, nil
}

func (s *shardedFlatContextStore) shardFor(name string) *flatContextStore {
	return s.shards[murmur3.Sum32([]byte(name))%s.n]
}

func (s *shardedFlatContextStore) maybeWrite(key uint64, name string, tags []string) error {
	return s.shardFor(name).maybeWrite(key, name, tags)
}

func (s *shardedFlatContextStore) scan(name string, filterTags []string) (map[uint64]contextEntry, error) {
	return s.shardFor(name).scan(name, filterTags)
}

func (s *shardedFlatContextStore) loadKeys(fn func(uint64)) error {
	for _, shard := range s.shards {
		if err := shard.loadKeys(fn); err != nil {
			return err
		}
	}
	return nil
}

func (s *shardedFlatContextStore) close() error {
	var last error
	for _, shard := range s.shards {
		if err := shard.close(); err != nil {
			last = err
		}
	}
	return last
}

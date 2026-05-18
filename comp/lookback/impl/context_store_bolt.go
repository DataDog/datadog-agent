// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"encoding/binary"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// boltContextStore is a contextStore backed by a bbolt B+tree database.
//
// Schema:
//
//	database: contexts.db
//	  bucket: "<metric_name>"
//	    key:   contextKey (uint64, big-endian 8 bytes)
//	    value: encodeTags(tags)
//
// One bucket per unique metric name. scan(name, tags) does a single O(log M)
// bucket lookup (M = unique metric names), then iterates the O(K) entries for
// that name (K = contexts per metric, typically 1-10).
//
// Writes are gated by the bloom filter in contextFile, so bbolt.Update is
// called only on first sight of a new context key — rare in steady state.
type boltContextStore struct {
	db *bolt.DB
}

func newBoltContextStore(path string) (*boltContextStore, error) {
	db, err := bolt.Open(path, 0o644, &bolt.Options{
		Timeout: 2 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("lookback bolt store open %s: %w", path, err)
	}
	return &boltContextStore{db: db}, nil
}

func (s *boltContextStore) maybeWrite(key uint64, name string, tags []string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(name))
		if err != nil {
			return err
		}
		var keyBuf [8]byte
		binary.BigEndian.PutUint64(keyBuf[:], key)
		return b.Put(keyBuf[:], encodeTags(tags))
	})
}

func (s *boltContextStore) scan(name string, filterTags []string) (map[uint64]contextEntry, error) {
	result := make(map[uint64]contextEntry)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(name))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			if len(k) < 8 {
				return nil
			}
			key := binary.BigEndian.Uint64(k)
			tags := decodeTags(v)
			if filterTags != nil && !tagsSubset(filterTags, tags) {
				return nil
			}
			result[key] = contextEntry{name: name, tags: tags}
			return nil
		})
	})
	return result, err
}

func (s *boltContextStore) loadKeys(fn func(uint64)) error {
	return s.db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(_ []byte, b *bolt.Bucket) error {
			return b.ForEach(func(k, _ []byte) error {
				if len(k) < 8 {
					return nil
				}
				fn(binary.BigEndian.Uint64(k))
				return nil
			})
		})
	})
}

func (s *boltContextStore) close() error {
	return s.db.Close()
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loopbackimpl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	loopback "github.com/DataDog/datadog-agent/comp/loopback/def"
)

// storeConfig holds the runtime configuration for a shardedStore.
type storeConfig struct {
	baseDir        string
	numShards      int
	windowDuration time.Duration
	maxAge         time.Duration
	maxDiskBytes   int64
	maxBufSize     int
}

// shardedStore manages N WAL shards, a background rotation goroutine,
// and the global context registry.
type shardedStore struct {
	cfg    storeConfig
	shards []*shard
	reg    *contextRegistry
	log    log.Component

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newShardedStore(cfg storeConfig, reg *contextRegistry, l log.Component) (*shardedStore, error) {
	if err := os.MkdirAll(cfg.baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("loopback store: mkdir %s: %w", cfg.baseDir, err)
	}
	now := time.Now().Unix()
	shards := make([]*shard, cfg.numShards)
	for i := range cfg.numShards {
		dir := filepath.Join(cfg.baseDir, fmt.Sprintf("shard-%03d", i))
		s, err := newShard(dir, now, cfg.maxBufSize, reg)
		if err != nil {
			// Best-effort cleanup of already-opened shards.
			for j := range i {
				_ = shards[j].stop()
			}
			return nil, err
		}
		shards[i] = s
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &shardedStore{
		cfg:    cfg,
		shards: shards,
		reg:    reg,
		log:    l,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// write fans out a single sample to the appropriate shard.
// Goroutine-safe: acquires the target shard's mutex internally.
func (ss *shardedStore) write(contextKey uint64, tsNs int64, value float64) {
	idx := int(contextKey % uint64(ss.cfg.numShards))
	s := ss.shards[idx]
	s.mu.Lock()
	_ = s.writeRecord(record{contextKey: contextKey, tsNs: tsNs, value: value})
	s.mu.Unlock()
}

// writeWithName is like write but also registers the context key / name / tags
// in the catalog if not already known.
func (ss *shardedStore) writeWithName(contextKey uint64, tsNs int64, value float64, name string, tags []string) {
	idx := int(contextKey % uint64(ss.cfg.numShards))
	s := ss.shards[idx]
	s.mu.Lock()
	_ = s.maybeRegisterKey(contextKey, name, tags)
	_ = s.writeRecord(record{contextKey: contextKey, tsNs: tsNs, value: value})
	s.mu.Unlock()
}

// startRotationTimer spawns the background goroutine that periodically rotates
// all shards and enforces retention.
func (ss *shardedStore) startRotationTimer() {
	ss.wg.Add(1)
	go ss.rotationLoop()
}

func (ss *shardedStore) rotationLoop() {
	defer ss.wg.Done()
	ticker := time.NewTicker(ss.cfg.windowDuration)
	defer ticker.Stop()
	for {
		select {
		case <-ss.ctx.Done():
			return
		case t := <-ticker.C:
			if err := ss.rotateAll(t.Unix()); err != nil {
				ss.log.Errorf("loopback: rotation error: %v", err)
			}
		}
	}
}

func (ss *shardedStore) rotateAll(newWindowStartSec int64) error {
	for _, s := range ss.shards {
		if err := s.rotate(newWindowStartSec); err != nil {
			ss.log.Errorf("loopback: shard rotate: %v", err)
		}
	}
	return ss.enforceRetention()
}

// enforceRetention removes files that are too old or push total bytes over the limit.
func (ss *shardedStore) enforceRetention() error {
	now := time.Now().Unix()
	cutoffSec := now - int64(ss.cfg.maxAge.Seconds())

	type shardFile struct {
		shardIdx int
		f        walFile
	}
	var surviving []shardFile
	var totalBytes int64

	for i, s := range ss.shards {
		files, err := s.sealedFiles()
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.windowStart < cutoffSec {
				_ = s.deleteFile(f.path)
				continue
			}
			surviving = append(surviving, shardFile{shardIdx: i, f: f})
			totalBytes += f.size
		}
	}

	// Sort surviving oldest first.
	for i := 1; i < len(surviving); i++ {
		for j := i; j > 0 && surviving[j].f.windowStart < surviving[j-1].f.windowStart; j-- {
			surviving[j], surviving[j-1] = surviving[j-1], surviving[j]
		}
	}
	for _, sf := range surviving {
		if totalBytes <= ss.cfg.maxDiskBytes {
			break
		}
		s := ss.shards[sf.shardIdx]
		if err := s.deleteFile(sf.f.path); err == nil {
			totalBytes -= sf.f.size
		}
	}
	return nil
}

// flush reads sealed WAL files for the given context keys and time range,
// then aggregates the records into Buckets.
func (ss *shardedStore) flush(ctx context.Context, keys []uint64, start, stop int64, intervalNs int64) ([]loopback.Bucket, error) {
	keySet := make(map[uint64]struct{}, len(keys))
	for _, k := range keys {
		keySet[k] = struct{}{}
	}

	windowSec := int64(ss.cfg.windowDuration.Seconds())
	var allRecs []record

	for _, s := range ss.shards {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		files, err := s.sealedFiles()
		if err != nil {
			continue
		}
		relevant := filesInRange(files, start, stop, windowSec)
		for _, f := range relevant {
			recs, err := readRecordsFromFile(f.path)
			if err != nil {
				ss.log.Warnf("loopback: read %s: %v", f.path, err)
				continue
			}
			allRecs = append(allRecs, recs...)
		}
	}

	buckets := aggregateRecords(allRecs, keySet, start, stop, intervalNs, func(k uint64) (string, []string, bool) {
		return ss.reg.getEntry(k)
	})
	return buckets, nil
}

// stop cancels the rotation goroutine, waits for it to exit, then flushes
// and closes all shard file handles.
func (ss *shardedStore) stop(_ context.Context) error {
	ss.cancel()
	ss.wg.Wait()
	for _, s := range ss.shards {
		if err := s.stop(); err != nil {
			ss.log.Warnf("loopback: shard stop: %v", err)
		}
	}
	return nil
}

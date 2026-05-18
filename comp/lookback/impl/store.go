// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	lookback "github.com/DataDog/datadog-agent/comp/lookback/def"
)

// storeConfig holds the runtime configuration for a shardedStore.
type storeConfig struct {
	baseDir        string
	numShards      int
	rotationInterval time.Duration
	maxAge         time.Duration
	maxDiskBytes   int64
	maxBufSize     int
}

// shardedStore manages N WAL shards and a background rotation goroutine.
// Context metadata is handled externally by contextFile.
type shardedStore struct {
	cfg    storeConfig
	shards []*shard
	log    log.Component

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newShardedStore(cfg storeConfig, l log.Component) (*shardedStore, error) {
	if err := os.MkdirAll(cfg.baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("lookback store: mkdir %s: %w", cfg.baseDir, err)
	}
	now := time.Now().Unix()
	shards := make([]*shard, cfg.numShards)
	for i := range cfg.numShards {
		dir := filepath.Join(cfg.baseDir, fmt.Sprintf("shard-%03d", i))
		s, err := newShard(dir, now, cfg.maxBufSize)
		if err != nil {
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
		log:    l,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// write fans out a single WAL record to the appropriate shard.
func (ss *shardedStore) write(contextKey uint64, tsNs int64, value float64) {
	idx := int(contextKey % uint64(ss.cfg.numShards))
	s := ss.shards[idx]
	s.mu.Lock()
	_ = s.writeRecord(record{contextKey: contextKey, tsNs: tsNs, value: value})
	s.mu.Unlock()
}

// startRotationTimer spawns the background rotation goroutine.
func (ss *shardedStore) startRotationTimer() {
	ss.wg.Add(1)
	go ss.rotationLoop()
}

func (ss *shardedStore) rotationLoop() {
	defer ss.wg.Done()
	ticker := time.NewTicker(ss.cfg.rotationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ss.ctx.Done():
			return
		case t := <-ticker.C:
			if err := ss.rotateAll(t.Unix()); err != nil {
				ss.log.Errorf("lookback: rotation error: %v", err)
			}
		}
	}
}

func (ss *shardedStore) rotateAll(newWindowStartSec int64) error {
	for _, s := range ss.shards {
		if err := s.rotate(newWindowStartSec); err != nil {
			ss.log.Errorf("lookback: shard rotate: %v", err)
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
// resolve maps a context key to (name, tags, ok) — supplied by the caller
// from contextFile.scan so the store has no knowledge of context metadata.
func (ss *shardedStore) flush(
	ctx context.Context,
	keys []uint64,
	start, stop int64,
	intervalNs int64,
	resolve func(uint64) (string, []string, bool),
) ([]lookback.Bucket, error) {
	keySet := make(map[uint64]struct{}, len(keys))
	for _, k := range keys {
		keySet[k] = struct{}{}
	}

	windowSec := int64(ss.cfg.rotationInterval.Seconds())
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
				ss.log.Warnf("lookback: read %s: %v", f.path, err)
				continue
			}
			allRecs = append(allRecs, recs...)
		}
	}

	return aggregateRecords(allRecs, keySet, start, stop, intervalNs, resolve), nil
}

// stop cancels the rotation goroutine, waits for it, then flushes all shards.
func (ss *shardedStore) stop(_ context.Context) error {
	ss.cancel()
	ss.wg.Wait()
	for _, s := range ss.shards {
		if err := s.stop(); err != nil {
			ss.log.Warnf("lookback: shard stop: %v", err)
		}
	}
	return nil
}

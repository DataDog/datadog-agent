// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"context"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	lookback "github.com/DataDog/datadog-agent/comp/lookback/def"
)

// walBackend wraps the sharded WAL store and bloom-filtered context file.
// It implements timeSeriesBackend.
type walBackend struct {
	store   *shardedStore
	ctxFile *contextFile
	log     log.Component
}

func newWALBackend(cfg storeConfig, l log.Component) (*walBackend, error) {
	ctxFile, err := newContextFile(cfg.baseDir)
	if err != nil {
		return nil, err
	}
	store, err := newShardedStore(cfg, l)
	if err != nil {
		_ = ctxFile.close()
		return nil, err
	}
	return &walBackend{store: store, ctxFile: ctxFile, log: l}, nil
}

func (b *walBackend) writeSample(name string, tags []string, tsUs int64, value float64) {
	ck := syntheticKey(name, tags)
	if err := b.ctxFile.write(ck, name, tags); err != nil {
		b.log.Warnf("lookback: context write error: %v", err)
	}
	b.store.write(ck, tsUs, value)
}

func (b *walBackend) flush(
	ctx context.Context, name string, tags []string,
	startUs, stopUs, intervalUs int64,
) ([]lookback.Bucket, error) {
	entries, err := b.ctxFile.scan(name, tags)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, lookback.ErrNoData
	}
	keys := make([]uint64, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	resolve := func(k uint64) (string, []string, bool) {
		e, ok := entries[k]
		return e.name, e.tags, ok
	}
	buckets, err := b.store.flush(ctx, keys, startUs, stopUs, intervalUs, resolve)
	if err != nil {
		return nil, err
	}
	if len(buckets) == 0 {
		return nil, lookback.ErrNoData
	}
	return buckets, nil
}

func (b *walBackend) startRotationTimer() {
	b.store.startRotationTimer()
}

func (b *walBackend) stop(ctx context.Context) error {
	storeErr := b.store.stop(ctx)
	ctxErr := b.ctxFile.close()
	if storeErr != nil {
		return storeErr
	}
	return ctxErr
}

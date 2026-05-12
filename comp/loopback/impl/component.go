// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package loopbackimpl implements the loopback ring buffer component.
package loopbackimpl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	loopback "github.com/DataDog/datadog-agent/comp/loopback/def"
	"github.com/DataDog/datadog-agent/pkg/hook"
)

// Requires defines the dependencies for the loopback component.
type Requires struct {
	fx.In

	Lc          fx.Lifecycle
	Config      config.Component
	Log         log.Component
	MetricHooks []hook.Hook[[]hook.MetricSampleSnapshot] `group:"hook"`
}

// Provides defines the output of the loopback component.
type Provides struct {
	fx.Out

	Comp loopback.Component
}

// NewComponent creates the loopback ring buffer component.
// When loopback.enabled is false a lightweight no-op implementation is returned.
func NewComponent(reqs Requires) (Provides, error) {
	if !reqs.Config.GetBool("loopback.enabled") {
		return Provides{Comp: &noopComponent{}}, nil
	}

	cfg := storeConfig{
		baseDir:        reqs.Config.GetString("loopback.dir"),
		numShards:      reqs.Config.GetInt("loopback.num_shards"),
		windowDuration: reqs.Config.GetDuration("loopback.window_duration"),
		maxAge:         reqs.Config.GetDuration("loopback.max_age"),
		maxDiskBytes:   reqs.Config.GetInt64("loopback.max_disk_bytes"),
		maxBufSize:     reqs.Config.GetInt("loopback.write_buffer_size"),
	}

	reqs.Log.Infof("loopback: initializing store at %s (shards=%d, window=%s, maxAge=%s, maxDisk=%dMB)",
		cfg.baseDir, cfg.numShards, cfg.windowDuration, cfg.maxAge, cfg.maxDiskBytes/1024/1024)

	reg := newContextRegistry()
	store, err := newShardedStore(cfg, reg, reqs.Log)
	if err != nil {
		return Provides{}, fmt.Errorf("loopback: init store: %w", err)
	}

	comp := &component{store: store, reg: reg, log: reqs.Log}

	var (
		unsubs []func()
		mu     sync.Mutex // guards unsubs
	)

	reqs.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			reqs.Log.Infof("loopback: subscribing to %d metric hook(s)", len(reqs.MetricHooks))
			pool := &sync.Pool{New: func() any {
				return make([]hook.MetricSampleSnapshot, 0, 64)
			}}
			for i, h := range reqs.MetricHooks {
				name := fmt.Sprintf("loopback-%d", i)
				unsub := h.Subscribe(name, comp.onSamples,
					hook.WithRecycle(
						func(src []hook.MetricSampleSnapshot) []hook.MetricSampleSnapshot {
							dst := pool.Get().([]hook.MetricSampleSnapshot)
							return append(dst[:0], src...)
						},
						func(b []hook.MetricSampleSnapshot) { pool.Put(b[:0]) },
					),
					hook.WithBufferSize[[]hook.MetricSampleSnapshot](1024),
				)
				mu.Lock()
				unsubs = append(unsubs, unsub)
				mu.Unlock()
			}
			store.startRotationTimer()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			mu.Lock()
			us := unsubs
			mu.Unlock()
			for _, u := range us {
				u()
			}
			return store.stop(ctx)
		},
	})

	return Provides{Comp: comp}, nil
}

// component is the enabled implementation of loopback.Component.
type component struct {
	store *shardedStore
	reg   *contextRegistry
	log   log.Component
}

func (c *component) onSamples(samples []hook.MetricSampleSnapshot) {
	for i := range samples {
		s := &samples[i]
		ck := s.ContextKey
		tsNs := int64(s.Timestamp * 1e9)
		if ck == 0 {
			ck = c.reg.register(s.Name, s.RawTags)
		}
		c.store.writeWithName(ck, tsNs, s.Value, s.Name, s.RawTags)
	}
}

func (c *component) Flush(ctx context.Context, name string, tags []string, start, stop int64, interval time.Duration) ([]loopback.Bucket, error) {
	keys := c.reg.lookupKeys(name, tags)
	if len(keys) == 0 {
		return nil, loopback.ErrNoData
	}
	intervalNs := int64(interval)
	buckets, err := c.store.flush(ctx, keys, start, stop, intervalNs)
	if err != nil {
		return nil, err
	}
	if len(buckets) == 0 {
		return nil, loopback.ErrNoData
	}
	return buckets, nil
}

// noopComponent is returned when loopback.enabled = false.
type noopComponent struct{}

func (n *noopComponent) Flush(_ context.Context, _ string, _ []string, _, _ int64, _ time.Duration) ([]loopback.Bucket, error) {
	return nil, loopback.ErrDisabled
}

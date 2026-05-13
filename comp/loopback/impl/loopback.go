// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package loopbackimpl implements the loopback ring buffer component.
package loopbackimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	loopback "github.com/DataDog/datadog-agent/comp/loopback/def"
	"github.com/DataDog/datadog-agent/pkg/hook"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
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

	Comp     loopback.Component
	Endpoint api.AgentEndpointProvider
}

// NewComponent creates the loopback ring buffer component.
// When loopback.enabled is false a lightweight no-op implementation is returned.
func NewComponent(reqs Requires) (Provides, error) {
	if !reqs.Config.GetBool("loopback.enabled") {
		noop := &noopComponent{}
		return Provides{
			Comp:     noop,
			Endpoint: api.NewAgentEndpointProvider(noop.handleFlush, "/loopback-flush", "GET"),
		}, nil
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

	ctxFile, err := newContextFile(filepath.Join(cfg.baseDir, "contexts.bin"))
	if err != nil {
		return Provides{}, fmt.Errorf("loopback: init context file: %w", err)
	}

	store, err := newShardedStore(cfg, reqs.Log)
	if err != nil {
		_ = ctxFile.close()
		return Provides{}, fmt.Errorf("loopback: init store: %w", err)
	}

	comp := &component{store: store, ctxFile: ctxFile, log: reqs.Log}

	var (
		unsubs []func()
		mu     sync.Mutex
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
			if err := store.stop(ctx); err != nil {
				return err
			}
			return ctxFile.close()
		},
	})

	return Provides{
		Comp:     comp,
		Endpoint: api.NewAgentEndpointProvider(comp.handleFlush, "/loopback-flush", "GET"),
	}, nil
}

// component is the enabled implementation of loopback.Component.
type component struct {
	store   *shardedStore
	ctxFile *contextFile
	log     log.Component
}

func (c *component) onSamples(samples []hook.MetricSampleSnapshot) {
	for i := range samples {
		s := &samples[i]
		ck := s.ContextKey
		if ck == 0 {
			// Check/no-aggr pipelines don't set ContextKey; compute a stable
			// synthetic key from name+tags without pre-registering in memory,
			// so ctxFile.maybeWrite owns both memory (bloom) and disk atomically.
			ck = syntheticKey(s.Name, sortedTagsCopy(s.RawTags))
		}
		_ = c.ctxFile.maybeWrite(ck, s.Name, s.RawTags)
		c.store.write(ck, int64(s.Timestamp*1e9), s.Value)
	}
}

func (c *component) Flush(ctx context.Context, name string, tags []string, start, stop int64, interval time.Duration) ([]loopback.Bucket, error) {
	entries, err := c.ctxFile.scan(name, tags)
	if err != nil {
		return nil, fmt.Errorf("loopback: context scan: %w", err)
	}
	if len(entries) == 0 {
		return nil, loopback.ErrNoData
	}

	keys := make([]uint64, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	resolve := func(k uint64) (string, []string, bool) {
		e, ok := entries[k]
		return e.name, e.tags, ok
	}

	intervalNs := int64(interval)
	buckets, err := c.store.flush(ctx, keys, start, stop, intervalNs, resolve)
	if err != nil {
		return nil, err
	}
	if len(buckets) == 0 {
		return nil, loopback.ErrNoData
	}
	return buckets, nil
}

// handleFlush serves GET /loopback-flush
//
// Query parameters:
//
//	name     — metric name (required)
//	tags     — comma-separated tag filter, e.g. env:prod,region:us (optional)
//	start    — range start, Unix nanoseconds (required)
//	stop     — range stop,  Unix nanoseconds (required)
//	interval — aggregation bucket width, e.g. 1s, 5s (optional, default 1s)
func (c *component) handleFlush(w http.ResponseWriter, r *http.Request) {
	buckets, err := parseAndFlush(r, c)
	if err != nil {
		httputils.SetJSONError(w, c.log.Errorf("loopback flush: %v", err), http.StatusBadRequest)
		return
	}
	out, err := json.Marshal(buckets)
	if err != nil {
		httputils.SetJSONError(w, c.log.Errorf("loopback flush marshal: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out) //nolint:errcheck
}

// noopComponent is returned when loopback.enabled = false.
type noopComponent struct{}

func (n *noopComponent) Flush(_ context.Context, _ string, _ []string, _, _ int64, _ time.Duration) ([]loopback.Bucket, error) {
	return nil, loopback.ErrDisabled
}

func (n *noopComponent) handleFlush(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, loopback.ErrDisabled.Error(), http.StatusServiceUnavailable)
}

func parseAndFlush(r *http.Request, comp loopback.Component) ([]loopback.Bucket, error) {
	q := r.URL.Query()

	name := q.Get("name")
	if name == "" {
		return nil, fmt.Errorf("missing required query parameter: name")
	}

	var tags []string
	if raw := q.Get("tags"); raw != "" {
		for _, t := range strings.Split(raw, ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
	}

	startNs, err := strconv.ParseInt(q.Get("start"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid start (want Unix nanoseconds): %w", err)
	}
	stopNs, err := strconv.ParseInt(q.Get("stop"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid stop (want Unix nanoseconds): %w", err)
	}

	var interval time.Duration
	if raw := q.Get("interval"); raw != "" {
		interval, err = time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid interval (want Go duration, e.g. 1s): %w", err)
		}
	}

	return comp.Flush(r.Context(), name, tags, startNs, stopNs, interval)
}

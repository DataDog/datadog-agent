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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	loopback "github.com/DataDog/datadog-agent/comp/loopback/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/hook"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	pkgserializer "github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// Requires defines the dependencies for the loopback component.
type Requires struct {
	fx.In

	Lc            fx.Lifecycle
	Config        config.Component
	Log           log.Component
	Hostname      hostnameinterface.Component
	Demultiplexer aggregator.Demultiplexer
	MetricHooks   []hook.Hook[[]hook.MetricSampleSnapshot] `group:"hook"`
}

// Provides defines the output of the loopback component.
type Provides struct {
	fx.Out

	Comp            loopback.Component
	FlushEndpoint   api.AgentEndpointProvider
	ForwardEndpoint api.AgentEndpointProvider
}

// NewComponent creates the loopback ring buffer component.
// When loopback.enabled is false a lightweight no-op implementation is returned.
func NewComponent(reqs Requires) (Provides, error) {
	if !reqs.Config.GetBool("loopback.enabled") {
		noop := &noopComponent{}
		return Provides{
			Comp:            noop,
			FlushEndpoint:   api.NewAgentEndpointProvider(noop.handleFlush, "/loopback-flush", "GET"),
			ForwardEndpoint: api.NewAgentEndpointProvider(noop.handleForward, "/loopback-forward", "POST"),
		}, nil
	}

	cfg := storeConfig{
		baseDir:          reqs.Config.GetString("loopback.dir"),
		numShards:        reqs.Config.GetInt("loopback.num_shards"),
		rotationInterval: reqs.Config.GetDuration("loopback.rotation_interval"),
		maxAge:           reqs.Config.GetDuration("loopback.max_age"),
		maxDiskBytes:     reqs.Config.GetInt64("loopback.max_disk_bytes"),
		maxBufSize:       reqs.Config.GetInt("loopback.write_buffer_size"),
	}

	reqs.Log.Infof("loopback: initializing store at %s (shards=%d, rotation=%s, maxAge=%s, maxDisk=%dMB)",
		cfg.baseDir, cfg.numShards, cfg.rotationInterval, cfg.maxAge, cfg.maxDiskBytes/1024/1024)

	if err := os.MkdirAll(cfg.baseDir, 0o755); err != nil {
		return Provides{}, fmt.Errorf("loopback: mkdir %s: %w", cfg.baseDir, err)
	}

	ctxFile, err := newContextFile(filepath.Join(cfg.baseDir, "contexts.bin"))
	if err != nil {
		return Provides{}, fmt.Errorf("loopback: init context file: %w", err)
	}

	store, err := newShardedStore(cfg, reqs.Log)
	if err != nil {
		_ = ctxFile.close()
		return Provides{}, fmt.Errorf("loopback: init store: %w", err)
	}

	comp := &component{
		store:      store,
		ctxFile:    ctxFile,
		log:        reqs.Log,
		serializer: reqs.Demultiplexer.Serializer(),
		hostname:   reqs.Hostname.GetSafe(context.Background()),
	}

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
		Comp:            comp,
		FlushEndpoint:   api.NewAgentEndpointProvider(comp.handleFlush, "/loopback-flush", "GET"),
		ForwardEndpoint: api.NewAgentEndpointProvider(comp.handleForward, "/loopback-forward", "POST"),
	}, nil
}

// component is the enabled implementation of loopback.Component.
type component struct {
	store      *shardedStore
	ctxFile    *contextFile
	log        log.Component
	serializer pkgserializer.MetricSerializer
	hostname   string
}

func (c *component) onSamples(samples []hook.MetricSampleSnapshot) {
	for i := range samples {
		s := &samples[i]
		ck := s.ContextKey
		if ck == 0 {
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

// flushParams holds parsed query parameters shared by /loopback-flush and /loopback-forward.
type flushParams struct {
	name     string
	tags     []string
	startNs  int64
	stopNs   int64
	interval time.Duration
}

func parseFlushParams(r *http.Request) (flushParams, error) {
	q := r.URL.Query()

	name := q.Get("name")
	if name == "" {
		return flushParams{}, fmt.Errorf("missing required query parameter: name")
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
		return flushParams{}, fmt.Errorf("invalid start (want Unix nanoseconds): %w", err)
	}
	stopNs, err := strconv.ParseInt(q.Get("stop"), 10, 64)
	if err != nil {
		return flushParams{}, fmt.Errorf("invalid stop (want Unix nanoseconds): %w", err)
	}

	var interval time.Duration
	if raw := q.Get("interval"); raw != "" {
		interval, err = time.ParseDuration(raw)
		if err != nil {
			return flushParams{}, fmt.Errorf("invalid interval (want Go duration, e.g. 1s): %w", err)
		}
	}

	return flushParams{name: name, tags: tags, startNs: startNs, stopNs: stopNs, interval: interval}, nil
}

// handleFlush serves GET /loopback-flush — returns aggregated buckets as JSON.
func (c *component) handleFlush(w http.ResponseWriter, r *http.Request) {
	p, err := parseFlushParams(r)
	if err != nil {
		httputils.SetJSONError(w, c.log.Errorf("loopback flush: %v", err), http.StatusBadRequest)
		return
	}
	buckets, err := c.Flush(r.Context(), p.name, p.tags, p.startNs, p.stopNs, p.interval)
	if err != nil {
		httputils.SetJSONError(w, c.log.Errorf("loopback flush: %v", err), http.StatusNotFound)
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

// handleForward serves POST /loopback-forward — reads WAL data and sends it to the backend.
//
// Same query parameters as /loopback-flush, plus:
//
//	mtype — "gauge" | "count" | "rate"  (default: gauge)
func (c *component) handleForward(w http.ResponseWriter, r *http.Request) {
	p, err := parseFlushParams(r)
	if err != nil {
		httputils.SetJSONError(w, c.log.Errorf("loopback forward: %v", err), http.StatusBadRequest)
		return
	}

	mtype := metrics.APIGaugeType
	switch r.URL.Query().Get("mtype") {
	case "count":
		mtype = metrics.APICountType
	case "rate":
		mtype = metrics.APIRateType
	}

	entries, err := c.ctxFile.scan(p.name, p.tags)
	if err != nil {
		httputils.SetJSONError(w, c.log.Errorf("loopback forward scan: %v", err), http.StatusInternalServerError)
		return
	}
	if len(entries) == 0 {
		httputils.SetJSONError(w, c.log.Errorf("loopback forward: %v", loopback.ErrNoData), http.StatusNotFound)
		return
	}

	keys := make([]uint64, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	resolve := func(k uint64) (string, []string, bool) {
		e, ok := entries[k]
		return e.name, e.tags, ok
	}

	buckets, err := c.store.flush(r.Context(), keys, p.startNs, p.stopNs, int64(p.interval), resolve)
	if err != nil {
		httputils.SetJSONError(w, c.log.Errorf("loopback forward flush: %v", err), http.StatusInternalServerError)
		return
	}
	if len(buckets) == 0 {
		httputils.SetJSONError(w, c.log.Errorf("loopback forward: %v", loopback.ErrNoData), http.StatusNotFound)
		return
	}

	host := c.hostname
	var sendErr error
	metrics.Serialize(
		metrics.NewIterableSeries(func(*metrics.Serie) {}, 100, 100),
		nil,
		func(serieSink metrics.SerieSink, _ metrics.SketchesSink) {
			for i := range buckets {
				b := &buckets[i]
				serieSink.Append(&metrics.Serie{
					Name:   b.Name,
					Points: []metrics.Point{{Ts: float64(b.Ts) / 1e9, Value: b.Sum}},
					Tags:   tagset.CompositeTagsFromSlice(b.Tags),
					Host:   host,
					MType:  mtype,
				})
			}
		},
		func(serieSource metrics.SerieSource) {
			sendErr = c.serializer.SendIterableSeries(serieSource)
		},
		nil,
	)

	if sendErr != nil {
		httputils.SetJSONError(w, c.log.Errorf("loopback forward send: %v", sendErr), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"forwarded":%d}`, len(buckets))
}

// noopComponent is returned when loopback.enabled = false.
type noopComponent struct{}

func (n *noopComponent) Flush(_ context.Context, _ string, _ []string, _, _ int64, _ time.Duration) ([]loopback.Bucket, error) {
	return nil, loopback.ErrDisabled
}

func (n *noopComponent) handleFlush(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, loopback.ErrDisabled.Error(), http.StatusServiceUnavailable)
}

func (n *noopComponent) handleForward(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, loopback.ErrDisabled.Error(), http.StatusServiceUnavailable)
}

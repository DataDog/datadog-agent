// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lookbackimpl implements the lookback ring buffer component.
package lookbackimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	lookback "github.com/DataDog/datadog-agent/comp/lookback/def"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	rcdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/hook"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	pkgserializer "github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// Requires defines the dependencies for the lookback component.
type Requires struct {
	fx.In

	Lc            fx.Lifecycle
	Config        config.Component
	Log           log.Component
	Hostname      hostnameinterface.Component
	Demultiplexer aggregator.Demultiplexer
	MetricHooks   []hook.Hook[[]hook.MetricSampleSnapshot] `group:"hook"`
}

// Provides defines the output of the lookback component.
type Provides struct {
	fx.Out

	Comp            lookback.Component
	FlushEndpoint   api.AgentEndpointProvider
	ForwardEndpoint api.AgentEndpointProvider
	RCListener      rcclienttypes.ListenerProvider
}

// NewComponent creates the lookback ring buffer component.
// When lookback.enabled is false a lightweight no-op implementation is returned.
func NewComponent(reqs Requires) (Provides, error) {
	if !reqs.Config.GetBool("lookback.enabled") {
		noop := &noopComponent{}
		return Provides{
			Comp:            noop,
			FlushEndpoint:   api.NewAgentEndpointProvider(noop.handleFlush, "/lookback-flush", "GET"),
			ForwardEndpoint: api.NewAgentEndpointProvider(noop.handleForward, "/lookback-forward", "POST"),
			RCListener:      rcclienttypes.ListenerProvider{ListenerProvider: rcclienttypes.RCListener{}},
		}, nil
	}

	cfg := storeConfig{
		baseDir:          reqs.Config.GetString("lookback.dir"),
		numShards:        reqs.Config.GetInt("lookback.num_shards"),
		rotationInterval: reqs.Config.GetDuration("lookback.rotation_interval"),
		maxAge:           reqs.Config.GetDuration("lookback.max_age"),
		maxDiskBytes:     reqs.Config.GetInt64("lookback.max_disk_bytes"),
		maxBufSize:       reqs.Config.GetInt("lookback.write_buffer_size"),
	}

	reqs.Log.Infof("lookback: initializing store at %s (shards=%d, rotation=%s, maxAge=%s, maxDisk=%dMB)",
		cfg.baseDir, cfg.numShards, cfg.rotationInterval, cfg.maxAge, cfg.maxDiskBytes/1024/1024)

	if err := os.MkdirAll(cfg.baseDir, 0o755); err != nil {
		return Provides{}, fmt.Errorf("lookback: mkdir %s: %w", cfg.baseDir, err)
	}

	backend, err := newBackend(cfg, reqs.Log)
	if err != nil {
		return Provides{}, fmt.Errorf("lookback: init backend: %w", err)
	}

	checkNames := reqs.Config.GetStringSlice("lookback.checks")
	checkInterval := reqs.Config.GetDuration("lookback.check_interval")

	comp := &component{
		backend:       backend,
		log:           reqs.Log,
		serializer:    reqs.Demultiplexer.Serializer(),
		hostname:      reqs.Hostname.GetSafe(context.Background()),
		checkNames:    checkNames,
		checkInterval: checkInterval,
	}

	var (
		unsubs []func()
		mu     sync.Mutex
	)

	reqs.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			reqs.Log.Infof("lookback: check runner config: checks=%v interval=%s",
				comp.checkNames, comp.checkInterval)
			comp.checkRunner = newLookbackCheckRunner(
				comp.checkNames, comp.checkInterval,
				comp.backend, comp.log,
			)

			reqs.Log.Infof("lookback: subscribing to %d metric hook(s)", len(reqs.MetricHooks))
			pool := &sync.Pool{New: func() any {
				return make([]hook.MetricSampleSnapshot, 0, 64)
			}}
			for i, h := range reqs.MetricHooks {
				name := fmt.Sprintf("lookback-%d", i)
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
			comp.backend.startRotationTimer()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			mu.Lock()
			us := unsubs
			mu.Unlock()
			for _, u := range us {
				u()
			}
			comp.checkRunner.stop()
			return comp.backend.stop(ctx)
		},
	})

	var rcListener rcclienttypes.ListenerProvider
	rcListener.ListenerProvider = rcclienttypes.RCListener{
		rcdata.ProductDebug: comp.onRCFlush,
	}

	return Provides{
		Comp:            comp,
		FlushEndpoint:   api.NewAgentEndpointProvider(comp.handleFlush, "/lookback-flush", "GET"),
		ForwardEndpoint: api.NewAgentEndpointProvider(comp.handleForward, "/lookback-forward", "POST"),
		RCListener:      rcListener,
	}, nil
}

// component is the enabled implementation of lookback.Component.
type component struct {
	backend       timeSeriesBackend
	log           log.Component
	serializer    pkgserializer.MetricSerializer
	hostname      string
	checkNames    []string
	checkInterval time.Duration
	checkRunner   *lookbackCheckRunner
}

func (c *component) onSamples(samples []hook.MetricSampleSnapshot) {
	for i := range samples {
		s := &samples[i]
		tags := s.RawTags
		if s.ContextKey != 0 {
			// Fast path: the pipeline already computed a context key, but we
			// still need name+tags for the context store. Use synthetic key so
			// write is idempotent across the two paths.
			tags = sortedTagsCopy(s.RawTags)
		} else {
			tags = sortedTagsCopy(s.RawTags)
		}
		c.backend.writeSample(s.Name, tags, int64(s.Timestamp*1e6), s.Value)
	}
}

func (c *component) Flush(ctx context.Context, name string, tags []string, start, stop int64, interval time.Duration) ([]lookback.Bucket, error) {
	intervalUs := int64(interval) / 1000
	return c.backend.flush(ctx, name, tags, start, stop, intervalUs)
}

// flushParams holds parsed query parameters shared by /lookback-flush and /lookback-forward.
type flushParams struct {
	name     string
	tags     []string
	startUs  int64
	stopUs   int64
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

	startUs, err := strconv.ParseInt(q.Get("start"), 10, 64)
	if err != nil {
		return flushParams{}, fmt.Errorf("invalid start (want Unix microseconds): %w", err)
	}
	stopUs, err := strconv.ParseInt(q.Get("stop"), 10, 64)
	if err != nil {
		return flushParams{}, fmt.Errorf("invalid stop (want Unix microseconds): %w", err)
	}

	var interval time.Duration
	if raw := q.Get("interval"); raw != "" {
		interval, err = time.ParseDuration(raw)
		if err != nil {
			return flushParams{}, fmt.Errorf("invalid interval (want Go duration, e.g. 1s): %w", err)
		}
	}

	return flushParams{name: name, tags: tags, startUs: startUs, stopUs: stopUs, interval: interval}, nil
}

// handleFlush serves GET /lookback-flush — returns aggregated buckets as JSON.
func (c *component) handleFlush(w http.ResponseWriter, r *http.Request) {
	p, err := parseFlushParams(r)
	if err != nil {
		httputils.SetJSONError(w, c.log.Errorf("lookback flush: %v", err), http.StatusBadRequest)
		return
	}
	buckets, err := c.Flush(r.Context(), p.name, p.tags, p.startUs, p.stopUs, p.interval)
	if err != nil {
		httputils.SetJSONError(w, c.log.Errorf("lookback flush: %v", err), http.StatusNotFound)
		return
	}
	out, err := json.Marshal(buckets)
	if err != nil {
		httputils.SetJSONError(w, c.log.Errorf("lookback flush marshal: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out) //nolint:errcheck
}

// handleForward serves POST /lookback-forward — reads WAL data and sends it to the backend.
//
// Same query parameters as /lookback-flush, plus:
//
//	mtype — "gauge" | "count" | "rate"  (default: gauge)
func (c *component) handleForward(w http.ResponseWriter, r *http.Request) {
	p, err := parseFlushParams(r)
	if err != nil {
		httputils.SetJSONError(w, c.log.Errorf("lookback forward: %v", err), http.StatusBadRequest)
		return
	}

	mtype := metrics.APIGaugeType
	switch r.URL.Query().Get("mtype") {
	case "count":
		mtype = metrics.APICountType
	case "rate":
		mtype = metrics.APIRateType
	}

	intervalUs := int64(p.interval) / 1000
	buckets, err := c.backend.flush(r.Context(), p.name, p.tags, p.startUs, p.stopUs, intervalUs)
	if err != nil {
		httputils.SetJSONError(w, c.log.Errorf("lookback forward: %v", err), http.StatusNotFound)
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
					Points: []metrics.Point{{Ts: float64(b.Ts) / 1e6, Value: b.Sum}},
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
		httputils.SetJSONError(w, c.log.Errorf("lookback forward send: %v", sendErr), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"forwarded":%d}`, len(buckets))
}

// rcFlushPayload is the JSON schema for a DEBUG Remote Config config that
// triggers a lookback flush and forwards the result to the Datadog backend.
type rcFlushPayload struct {
	Name       string `json:"name"`
	Tags       string `json:"tags"`        // comma-separated, optional
	Start      int64  `json:"start"`       // Unix microseconds
	Stop       int64  `json:"stop"`        // Unix microseconds
	IntervalMs int64  `json:"interval_ms"` // milliseconds, optional (default: 1000 = 1s)
}

// onRCFlush is registered as a DEBUG product RCListener callback. When the
// backend pushes a DEBUG config containing a rcFlushPayload, this method runs
// the flush+forward pipeline and reports the apply status back to RC.
func (c *component) onRCFlush(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	for configPath, raw := range updates {
		var p rcFlushPayload
		if err := json.Unmarshal(raw.Config, &p); err != nil {
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: fmt.Sprintf("invalid payload: %v", err),
			})
			continue
		}

		var tags []string
		if p.Tags != "" {
			for _, t := range strings.Split(p.Tags, ",") {
				if t = strings.TrimSpace(t); t != "" {
					tags = append(tags, t)
				}
			}
		}

		var intervalUs int64
		if p.IntervalMs > 0 {
			intervalUs = p.IntervalMs * 1000 // ms → µs
		}

		buckets, err := c.backend.flush(context.Background(), p.Name, tags, p.Start, p.Stop, intervalUs)
		if err != nil {
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: fmt.Sprintf("flush error for %q: %v", p.Name, err),
			})
			continue
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
						Points: []metrics.Point{{Ts: float64(b.Ts) / 1e6, Value: b.Sum}},
						Tags:   tagset.CompositeTagsFromSlice(b.Tags),
						Host:   host,
						MType:  metrics.APIGaugeType,
					})
				}
			},
			func(src metrics.SerieSource) { sendErr = c.serializer.SendIterableSeries(src) },
			nil,
		)

		if sendErr != nil {
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: sendErr.Error(),
			})
		} else {
			c.log.Infof("lookback: RC flush forwarded %d buckets for %q", len(buckets), p.Name)
			applyStateCallback(configPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
	}
}

// noopComponent is returned when lookback.enabled = false.
type noopComponent struct{}

func (n *noopComponent) Flush(_ context.Context, _ string, _ []string, _, _ int64, _ time.Duration) ([]lookback.Bucket, error) {
	return nil, lookback.ErrDisabled
}

func (n *noopComponent) handleFlush(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, lookback.ErrDisabled.Error(), http.StatusServiceUnavailable)
}

func (n *noopComponent) handleForward(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, lookback.ErrDisabled.Error(), http.StatusServiceUnavailable)
}

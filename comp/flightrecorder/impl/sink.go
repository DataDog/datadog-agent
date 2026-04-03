// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	flightrecorder "github.com/DataDog/datadog-agent/comp/flightrecorder/def"
	"github.com/DataDog/datadog-agent/pkg/hook"
)

// tagPool recycles string slices used for metric/log tag copies.
// Pre-capped to 32 strings to cover the common case without re-allocation.
var tagPool = sync.Pool{New: func() any { s := make([]string, 0, 32); return &s }}

// contentPool recycles byte slices used for log content copies.
var contentPool = sync.Pool{New: func() any { s := make([]byte, 0, 256); return &s }}

// Requires defines the dependencies injected into the flight recorder component.
type Requires struct {
	Lc     compdef.Lifecycle
	Config config.Component
	Log    log.Component

	MetricsHooks    []hook.Hook[[]hook.MetricSampleSnapshot] `group:"hook"`
	LogsHooks       []hook.Hook[[]hook.LogSampleSnapshot]    `group:"hook"`
	TraceStatsHooks []hook.Hook[hook.TraceStatsView]         `group:"hook"`
}

// Provides defines what the component exposes to the fx graph.
type Provides struct {
	Comp flightrecorder.Component
}

// sinkImpl implements flightrecorder.Component with socket-based auto-activation.
// It probes for the sidecar's Unix socket and activates when found. On disconnect,
// it tears down hooks and batcher, then restarts discovery.
type sinkImpl struct {
	counters *counters
	log      log.Component

	// Config values (read once in NewComponent).
	socketPath         string
	flushInterval      time.Duration
	ptCapacity         int
	defCapacity        int
	logCapacity        int
	traceStatsCapacity int
	hookBufSize        int
	contextCap         int

	// Context deduplication — persists across reconnect cycles so the agent
	// doesn't re-send all 50K+ context definitions on every reconnect.
	// Context keys are deterministic hashes, so the sidecar's bloom filter
	// deduplicates any contexts that were already persisted to contexts.bin.
	seenContexts *contextSet

	// Hooks (injected, immutable).
	metricsHooks    []hook.Hook[[]hook.MetricSampleSnapshot]
	logsHooks       []hook.Hook[[]hook.LogSampleSnapshot]
	traceStatsHooks []hook.Hook[hook.TraceStatsView]

	// Lifecycle.
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func (s *sinkImpl) Stats() flightrecorder.Stats {
	return s.counters.stats()
}

// NewComponent creates the flight recorder fx component. The component probes
// for the sidecar's Unix socket with exponential backoff and activates
// automatically when the socket is found.
func NewComponent(req Requires) (Provides, error) {
	socketPath := req.Config.GetString("flightrecorder.socket_path")
	flushInterval := req.Config.GetDuration("flightrecorder.flush_interval")
	ptCapacity := req.Config.GetInt("flightrecorder.point_buffer_capacity")
	defCapacity := req.Config.GetInt("flightrecorder.def_buffer_capacity")
	logCapacity := req.Config.GetInt("flightrecorder.log_buffer_capacity")
	traceStatsCapacity := req.Config.GetInt("flightrecorder.trace_stats_buffer_capacity")
	hookBufSize := req.Config.GetInt("flightrecorder.hook_buffer_size")
	contextCap := req.Config.GetInt("flightrecorder.context_set_capacity")

	ctx, cancel := context.WithCancel(context.Background())
	impl := &sinkImpl{
		counters:           &counters{},
		log:                req.Log,
		socketPath:         socketPath,
		flushInterval:      flushInterval,
		ptCapacity:         ptCapacity,
		defCapacity:        defCapacity,
		logCapacity:        logCapacity,
		traceStatsCapacity: traceStatsCapacity,
		hookBufSize:        hookBufSize,
		contextCap:         contextCap,
		seenContexts:       newContextSet(contextCap),
		metricsHooks:       req.MetricsHooks,
		logsHooks:          req.LogsHooks,
		traceStatsHooks:    req.TraceStatsHooks,
		cancel:             cancel,
	}

	req.Lc.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			impl.wg.Add(1)
			go impl.discoveryLoop(ctx)
			return nil
		},
		OnStop: func(_ context.Context) error {
			cancel()
			impl.wg.Wait()
			return nil
		},
	})

	return Provides{Comp: impl}, nil
}

// discoveryLoop probes the sidecar socket with exponential backoff.
// On success, it activates (subscribes hooks, creates batcher+transport).
// When the connection is lost, it tears down and restarts discovery.
func (s *sinkImpl) discoveryLoop(ctx context.Context) {
	defer s.wg.Done()
	for {
		bo := backoff.NewExponentialBackOff()
		bo.InitialInterval = 100 * time.Millisecond
		bo.MaxInterval = 30 * time.Second
		bo.RandomizationFactor = 0.1
		bo.Multiplier = 2.0
		_, err := backoff.Retry(ctx, func() (struct{}, error) {
			conn, err := net.DialTimeout("unix", s.socketPath, time.Second)
			if err != nil {
				return struct{}{}, err
			}
			conn.Close()
			return struct{}{}, nil
		}, backoff.WithBackOff(bo))
		if err != nil {
			return // context cancelled
		}

		s.log.Infof("flightrecorder: socket discovered at %s, activating", s.socketPath)
		done := s.activate(ctx)

		select {
		case <-ctx.Done():
			return
		case <-done:
			s.log.Infof("flightrecorder: deactivated, restarting discovery")
		}
	}
}

// activate creates the transport, batcher, and subscribes to all hooks.
// It returns a channel that is closed when the transport disconnects,
// signaling the discovery loop to restart.
func (s *sinkImpl) activate(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})

	c := s.counters
	transport := newUnixTransport(ctx, s.socketPath)
	bat := newBatcher(transport, s.flushInterval,
		s.ptCapacity, s.defCapacity, s.logCapacity, s.traceStatsCapacity, s.seenContexts, c)

	// Subscribe to all hooks, collect unsubscribe functions.
	var unsubs []func()

	metricBatchPool := sync.Pool{New: func() any {
		return make([]hook.MetricSampleSnapshot, 0, 64)
	}}
	for _, mh := range s.metricsHooks {
		if mh == nil {
			continue
		}
		source := mh.Name()
		unsub := mh.Subscribe("flightrecorder-metrics", func(batch []hook.MetricSampleSnapshot) {
			go func() {
				c.incHookCallbacks(uint64(len(batch)))
				for i := range batch {
					ms := &batch[i]
					ckey := ms.ContextKey
					ts := int64(ms.Timestamp * float64(time.Second/time.Nanosecond))

					if bat.IsContextKnown(ckey) {
						bat.AddPoint(metricPoint{
							ContextKey:  ckey,
							Value:       ms.Value,
							TimestampNs: ts,
							SampleRate:  ms.SampleRate,
							Source:      source,
						})
					} else {
						bat.AddContextDef(contextDef{
							ContextKey:   ckey,
							Name:         ms.Name,
							Value:        ms.Value,
							Tags:         ms.RawTags,
							TagPoolSlice: nil,
							TimestampNs:  ts,
							SampleRate:   ms.SampleRate,
							Source:       source,
						})
					}
				}
			}()
		},
			hook.WithBufferSize[[]hook.MetricSampleSnapshot](s.hookBufSize),
			hook.WithRecycle(
				func(src []hook.MetricSampleSnapshot) []hook.MetricSampleSnapshot {
					dst := metricBatchPool.Get().([]hook.MetricSampleSnapshot)
					return append(dst[:0], src...)
				},
				func(b []hook.MetricSampleSnapshot) { metricBatchPool.Put(b[:0]) },
			),
		)
		unsubs = append(unsubs, unsub)
	}

	for _, lh := range s.logsHooks {
		if lh == nil {
			continue
		}
		unsub := lh.Subscribe("flightrecorder-logs", func(batch []hook.LogSampleSnapshot) {
			bat.AddLogBatch(batch)
		}, hook.WithBufferSize[[]hook.LogSampleSnapshot](s.hookBufSize))
		unsubs = append(unsubs, unsub)
	}

	for _, sh := range s.traceStatsHooks {
		if sh == nil {
			continue
		}
		unsub := sh.Subscribe("flightrecorder-trace-stats", func(payload hook.TraceStatsView) {
			bat.AddTraceStat(capturedTraceStat{
				Service:          payload.GetService(),
				Name:             payload.GetName(),
				Resource:         payload.GetResource(),
				Type:             payload.GetType(),
				SpanKind:         payload.GetSpanKind(),
				HTTPStatusCode:   payload.GetHTTPStatusCode(),
				Hits:             payload.GetHits(),
				Errors:           payload.GetErrors(),
				DurationNs:       payload.GetDuration(),
				TopLevelHits:     payload.GetTopLevelHits(),
				OkSummary:        payload.GetOkSummary(),
				ErrorSummary:     payload.GetErrorSummary(),
				Hostname:         payload.GetHostname(),
				Env:              payload.GetEnv(),
				Version:          payload.GetVersion(),
				BucketStartNs:    int64(payload.GetBucketStartNs()),
				BucketDurationNs: int64(payload.GetBucketDurationNs()),
				TimestampNs:      time.Now().UnixNano(),
			})
		}, hook.WithBufferSize[hook.TraceStatsView](s.hookBufSize))
		unsubs = append(unsubs, unsub)
	}

	s.log.Infof("flightrecorder: activated (socket=%s flush=%s pts=%d defs=%d logs=%d tss=%d hook_buf=%d ctx_cap=%d)",
		s.socketPath, s.flushInterval, s.ptCapacity, s.defCapacity, s.logCapacity, s.traceStatsCapacity, s.hookBufSize, s.contextCap)

	// Set transport disconnect handler: tear down everything and signal discovery loop.
	transport.onDisconnect = func() {
		for _, unsub := range unsubs {
			unsub()
		}
		bat.Stop()
		close(done)
	}

	return done
}

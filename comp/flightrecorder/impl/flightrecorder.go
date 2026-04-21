// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"context"
	"hash/fnv"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"
	flatbuffers "github.com/google/flatbuffers/go"

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

// flightrecorderImpl implements flightrecorder.Component with socket-based auto-activation.
// It probes for the sidecar's Unix socket and activates when found. On disconnect,
// it tears down hooks and batcher, then restarts discovery.
type flightrecorderImpl struct {
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
	impl := &flightrecorderImpl{
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
func (s *flightrecorderImpl) discoveryLoop(ctx context.Context) {
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
func (s *flightrecorderImpl) activate(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	c := s.counters
	pool := newBuilderPool()

	// Create 3 independent pipelines, each with its own UDS connection.
	metricsTransport := newUnixConn(s.socketPath)
	logsTransport := newUnixConn(s.socketPath)
	traceTransport := newUnixConn(s.socketPath)

	metricsPipe := newPipeline[metricPoint](
		metricsTransport, s.flushInterval, s.ptCapacity, s.defCapacity,
		func(p *builderPool, buf []metricPoint, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodePointBatch(p, buf, tail, count, cap)
		},
		func(p *builderPool, buf []contextDef, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodeContextBatch(p, buf, tail, count, cap)
		},
		"metrics", c.incMetricsSent, c.incMetricsDroppedOverflow, c.incMetricsDroppedTransport,
		pool, c,
	)

	logsPipe := newPipeline[logEntry](
		logsTransport, s.flushInterval, s.logCapacity, s.defCapacity,
		func(p *builderPool, buf []logEntry, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodeLogEntryBatch(p, buf, tail, count, cap)
		},
		func(p *builderPool, buf []contextDef, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodeLogContextBatch(p, buf, tail, count, cap)
		},
		"logs", c.incLogsSent, c.incLogsDroppedOverflow, c.incLogsDroppedTransport,
		pool, c,
	)

	tracePipe := newPipeline[capturedTraceStat](
		traceTransport, s.flushInterval, s.traceStatsCapacity, 0,
		func(p *builderPool, buf []capturedTraceStat, tail, count, cap int) (*flatbuffers.Builder, error) {
			return EncodeTraceStatsBatchRing(p, buf, tail, count, cap)
		},
		nil, // no context ring for trace stats
		"trace_stats", c.incTraceStatsSent, c.incTraceStatsDroppedOverflow, c.incTraceStatsDroppedTransport,
		pool, c,
	)

	// Any transport disconnect tears down everything exactly once.
	var disconnectOnce sync.Once
	var unsubs []func()
	teardown := func() {
		disconnectOnce.Do(func() {
			for _, unsub := range unsubs {
				unsub()
			}
			metricsPipe.Stop()
			logsPipe.Stop()
			tracePipe.Stop()
			metricsTransport.Close() //nolint:errcheck
			logsTransport.Close()    //nolint:errcheck
			traceTransport.Close()   //nolint:errcheck
			close(done)
		})
	}
	metricsTransport.onDisconnect = teardown
	logsTransport.onDisconnect = teardown
	traceTransport.onDisconnect = teardown

	// Subscribe to all hooks.
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
					if ckey == 0 && ms.Name != "" {
						ckey = computeContextKey(ms.Name, ms.RawTags)
					}
					ts := int64(ms.Timestamp * float64(time.Second/time.Nanosecond))

					metricsPipe.AddEntry(metricPoint{
						ContextKey:  ckey,
						Value:       ms.Value,
						TimestampNs: ts,
						SampleRate:  ms.SampleRate,
					})
					if !s.seenContexts.IsKnown(ckey) {
						metricsPipe.AddContextDef(contextDef{
							ContextKey: ckey,
							Name:       ms.Name,
							Tags:       ms.RawTags,
							Source:     source,
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
			for i := range batch {
				lg := &batch[i]
				allTags := buildLogContextTags(lg.Hostname, lg.Status, lg.Tags)
				ckey := computeContextKey("", allTags)
				if !s.seenContexts.IsKnown(ckey) {
					logsPipe.AddContextDef(contextDef{
						ContextKey: ckey,
						Tags:       allTags,
					})
				}
				logsPipe.AddEntry(logEntry{
					ContextKey:  ckey,
					Content:     lg.Content,
					TimestampNs: lg.TimestampNs,
				})
			}
		}, hook.WithBufferSize[[]hook.LogSampleSnapshot](s.hookBufSize))
		unsubs = append(unsubs, unsub)
	}

	for _, sh := range s.traceStatsHooks {
		if sh == nil {
			continue
		}
		unsub := sh.Subscribe("flightrecorder-trace-stats", func(payload hook.TraceStatsView) {
			tracePipe.AddEntry(capturedTraceStat{
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

	s.log.Infof("flightrecorder: activated (socket=%s flush=%s metrics=[pts=%d defs=%d] logs=%d trace_stats=%d hook_buf=%d ctx_cap=%d)",
		s.socketPath, s.flushInterval, s.ptCapacity, s.defCapacity, s.logCapacity, s.traceStatsCapacity, s.hookBufSize, s.contextCap)

	return done
}

// computeContextKey produces a deterministic 64-bit key from a metric name
// and its tags. Used for check metrics where the aggregator's ContextKey is 0.
// Tags are sorted before hashing for stability.
func computeContextKey(name string, tags []string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(name))
	h.Write([]byte{0}) // separator
	sorted := make([]string, len(tags))
	copy(sorted, tags)
	sort.Strings(sorted)
	for _, t := range sorted {
		h.Write([]byte(t))
		h.Write([]byte{0})
	}
	return h.Sum64()
}

// buildLogContextTags folds hostname and status into the tag list so that
// log contexts use the same ContextEntry format as metrics.
func buildLogContextTags(hostname, status string, tags []string) []string {
	allTags := make([]string, 0, len(tags)+2)
	if hostname != "" {
		allTags = append(allTags, "hostname:"+hostname)
	}
	if status != "" {
		allTags = append(allTags, "status:"+status)
	}
	allTags = append(allTags, tags...)
	return allTags
}

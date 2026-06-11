// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"cmp"
	"slices"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	pkgmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	statefulgrpc "github.com/DataDog/datadog-agent/pkg/serializer/grpc"
)

// Router maps a series to a lane index by a stable key, deterministically
// within a flush.
type Router interface {
	Route(serie *pkgmetrics.Serie, numLanes int) int
}

// identityRouter routes every series to lane 0.
type identityRouter struct{}

func (identityRouter) Route(_ *pkgmetrics.Serie, _ int) int { return 0 }

// statefulLane is one unit of the stateful path: a StreamDictionary and a
// PayloadSink, with a fresh encoder bound to them each flush. Each lane has its
// own dictionary — a dictionary cannot be sharded across streams (a Define on
// one stream referenced from another would break the receiver), so parallelism
// means more lanes, not one dictionary split across streams.
type statefulLane struct {
	dict *StreamDictionary
	sink statefulgrpc.PayloadSink
}

// StatefulOutput is the per-serializer handle for the series stateful path,
// owning the lane pool, the router, and the destination Senders. It is never
// shared between serializers, so concurrent producers never mutate one
// dictionary. The lane set is fixed during a flush; resize is guarded by mu.
type StatefulOutput struct {
	mu     sync.RWMutex
	lanes  []*statefulLane
	router Router

	// senders is every destination Sender across all lanes, retained for
	// lifecycle (Start/Stop); PayloadSink has no lifecycle methods.
	senders []*statefulgrpc.Sender
}

// NewStatefulOutput builds a single-lane output whose lane fans out to the given
// destination Senders. The caller constructs the Senders (config-driven).
func NewStatefulOutput(dests []*statefulgrpc.Sender) *StatefulOutput {
	lane := &statefulLane{
		dict: NewStreamDictionary(),
		sink: statefulgrpc.NewFanout(dests),
	}
	return &StatefulOutput{
		lanes:   []*statefulLane{lane},
		router:  identityRouter{},
		senders: dests,
	}
}

// Start spins up every destination Sender. Idempotent per Sender.
func (o *StatefulOutput) Start() {
	for _, s := range o.senders {
		s.Start()
	}
}

// Stop halts every destination Sender.
func (o *StatefulOutput) Stop() {
	for _, s := range o.senders {
		s.Stop()
	}
}

// flushView returns the lane set + router for one flush, under RLock so a
// concurrent resize can't change the lane count mid-flush.
func (o *StatefulOutput) flushView() ([]*statefulLane, Router) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.lanes, o.router
}

// statefulFlushWriter is the per-flush serieWriter for the stateful path. It
// routes each series to a lane and buffers it (routing is pre-encode because
// each lane interns against its own dictionary); finishPayload then pre-interns
// tagsets, sorts by tagset id, and encodes each lane's subset.
type statefulFlushWriter struct {
	config          config.Component
	strategy        compression.Component
	pipelineConfig  PipelineConfig
	pipelineContext *PipelineContext

	lanes   []*statefulLane
	router  Router
	buffers [][]*pkgmetrics.Serie
}

// newStatefulFlushWriter snapshots the output's lanes for this flush and
// allocates a per-lane buffer.
func newStatefulFlushWriter(
	cfg config.Component,
	strategy compression.Component,
	pipelineConfig PipelineConfig,
	pipelineContext *PipelineContext,
	output *StatefulOutput,
) *statefulFlushWriter {
	lanes, router := output.flushView()
	return &statefulFlushWriter{
		config:          cfg,
		strategy:        strategy,
		pipelineConfig:  pipelineConfig,
		pipelineContext: pipelineContext,
		lanes:           lanes,
		router:          router,
		buffers:         make([][]*pkgmetrics.Serie, len(lanes)),
	}
}

func (w *statefulFlushWriter) startPayload() error { return nil }

// writeSerie routes and buffers; it does not encode. Encoding is deferred to
// finishPayload so each lane can pre-intern + sort its own subset.
func (w *statefulFlushWriter) writeSerie(serie *pkgmetrics.Serie) error {
	idx := w.router.Route(serie, len(w.lanes))
	w.buffers[idx] = append(w.buffers[idx], serie)
	return nil
}

// finishPayload encodes each lane's buffered subset against that lane's
// dictionary and submits to that lane's sink.
func (w *statefulFlushWriter) finishPayload() error {
	for i, lane := range w.lanes {
		if len(w.buffers[i]) == 0 {
			continue
		}
		enc, err := newPayloadsBuilderV3StatefulWithConfig(
			w.config, w.strategy, w.pipelineConfig, w.pipelineContext, lane.dict, lane.sink,
		)
		if err != nil {
			return err
		}
		if err := encodeSortedSubset(enc, w.buffers[i]); err != nil {
			return err
		}
		if err := enc.finishPayload(); err != nil {
			return err
		}
	}
	return nil
}

// encodeSortedSubset pre-interns each series' tagset into the encoder's
// dictionary (assigning/warming IDs and front-loading any new tagset Defines
// into the first batch), sorts the subset by tagset id ascending so tagset-ref
// deltas are sequential (~1 B/ref), then writes the series in that order.
func encodeSortedSubset(enc *payloadsBuilderV3Stateful, series []*pkgmetrics.Serie) error {
	tagsetIDs := make([]uint64, len(series))
	for i, serie := range series {
		tagsetIDs[i] = enc.dict.InternTags(serie.Tags, &enc.defines)
	}
	indices := make([]int, len(series))
	for i := range indices {
		indices[i] = i
	}
	slices.SortStableFunc(indices, func(a, b int) int {
		return cmp.Compare(tagsetIDs[a], tagsetIDs[b])
	})
	for _, idx := range indices {
		if err := enc.writeSerie(series[idx]); err != nil {
			return err
		}
	}
	return nil
}

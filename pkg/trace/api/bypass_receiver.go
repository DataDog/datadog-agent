// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	apiheader "github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// BypassReceiver is a minimal Receiver implementation which does not expose HTTP endpoints.
// It provides public methods to submit raw trace and stats payloads as byte slices, and forwards
// decoded Payloads to the processing pipeline via the output channel.
type BypassReceiver struct {
	stats *info.ReceiverStats

	out                 chan *Payload
	conf                *config.AgentConfig
	statsProcessor      StatsProcessor
	containerIDProvider IDProvider

	statsd statsd.ClientInterface
}

// NewBypassReceiver creates a new BypassReceiver.
func NewBypassReceiver(conf *config.AgentConfig, out chan *Payload, statsProcessor StatsProcessor, statsd statsd.ClientInterface) *BypassReceiver {
	return &BypassReceiver{
		stats:               info.NewReceiverStats(),
		out:                 out,
		conf:                conf,
		statsProcessor:      statsProcessor,
		containerIDProvider: NewIDProvider(conf.ContainerProcRoot, conf.ContainerIDFromOriginInfo),
		statsd:              statsd,
	}
}

// Start implements Receiver; no-op for BypassReceiver.
func (*BypassReceiver) Start() {}

// Stop implements Receiver; no-op for BypassReceiver.
func (*BypassReceiver) Stop() error { return nil }

// BuildHandlers implements Receiver; no-op for BypassReceiver.
func (*BypassReceiver) BuildHandlers() {}

// UpdateAPIKey implements Receiver; no-op for BypassReceiver.
func (*BypassReceiver) UpdateAPIKey() {}

// Languages implements Receiver using the same logic as HTTPReceiver.
func (r *BypassReceiver) Languages() string {
	langs := make(map[string]bool)
	list := []string{}
	r.stats.RLock()
	for tags := range r.stats.Stats {
		if _, ok := langs[tags.Lang]; !ok {
			list = append(list, tags.Lang)
			langs[tags.Lang] = true
		}
	}
	r.stats.RUnlock()
	// preserve sorting behavior for determinism
	sortStrings(list)
	return joinStrings(list, "|")
}

// GetStats implements Receiver and returns receiver stats.
func (r *BypassReceiver) GetStats() *info.ReceiverStats { return r.stats }

// GetHandler implements Receiver; BypassReceiver does not expose handlers.
func (*BypassReceiver) GetHandler(pattern string) (http.Handler, bool) { return nil, false }

// SubmitTraces ingests a raw tracer payload body and forwards a built Payload to the output channel.
// Headers should contain the same metadata as HTTP requests (e.g., Datadog-Meta-* headers).
func (r *BypassReceiver) SubmitTraces(ctx context.Context, v Version, headers map[string]string, body []byte) error {
	// Build a lightweight http.Header substitute
	h := make(map[string][]string, len(headers))
	for k, v := range headers {
		h[k] = []string{v}
	}
	// Trace count, if provided
	var tracen int64
	if s := headers[apiheader.TraceCount]; s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			tracen = int64(n)
		}
	}
	start := time.Now()
	cid := r.containerIDProvider.GetContainerID(ctx, h)
	payload, ts, err := BuildPayloadAndRecordStats(
		v,
		GetMediaTypeValue(headers["Content-Type"]),
		body,
		cid,
		getHeader(headers, apiheader.Lang),
		getHeader(headers, apiheader.LangVersion),
		getHeader(headers, apiheader.LangInterpreter),
		getHeader(headers, apiheader.LangInterpreterVendor),
		getHeader(headers, apiheader.TracerVersion),
		getHeader(headers, apiheader.ProcessTags),
		r.conf.ContainerTags,
		func(t info.Tags) *info.TagStats { return r.stats.GetTagStats(t) },
		tracen,
		r.statsd,
		start,
	)
	if err != nil {
		return err
	}
	// set client flags from headers
	payload.ClientComputedTopLevel = isHeaderTrue(apiheader.ComputedTopLevel, getHeader(headers, apiheader.ComputedTopLevel))
	payload.ClientComputedStats = isHeaderTrue(apiheader.ComputedStats, getHeader(headers, apiheader.ComputedStats))
	payload.ClientDroppedP0s = droppedTracesFromHeader(h, ts)
	r.out <- payload
	return nil
}

// SubmitStats ingests a raw stats payload body and forwards it to the configured StatsProcessor.
func (r *BypassReceiver) SubmitStats(ctx context.Context, headers map[string]string, body []byte) error {
	// Build a lightweight http.Header substitute
	h := make(map[string][]string, len(headers))
	for k, v := range headers {
		h[k] = []string{v}
	}
	return DecodeAndProcessClientStats(
		ctx,
		body,
		getHeader(headers, apiheader.Lang),
		getHeader(headers, apiheader.LangVersion),
		getHeader(headers, apiheader.LangInterpreter),
		getHeader(headers, apiheader.LangInterpreterVendor),
		getHeader(headers, apiheader.TracerVersion),
		getHeader(headers, apiheader.TracerObfuscationVersion),
		r.containerIDProvider.GetContainerID(ctx, h),
		func(t info.Tags) *info.TagStats { return r.stats.GetTagStats(t) },
		r.statsd,
		r.statsProcessor,
	)
}

// helpers
func getHeader(h map[string]string, key string) string { return h[key] }

// sortStrings sorts in place; placed here to avoid importing sort just for a small helper
func sortStrings(a []string) {
	if len(a) < 2 {
		return
	}
	// simple insertion sort (n is small)
	for i := 1; i < len(a); i++ {
		j := i
		for j > 0 && a[j-1] > a[j] {
			a[j-1], a[j] = a[j], a[j-1]
			j--
		}
	}
}

func joinStrings(a []string, sep string) string {
	if len(a) == 0 {
		return ""
	}
	n := 0
	for _, s := range a {
		n += len(s)
	}
	n += len(sep) * (len(a) - 1)
	b := make([]byte, 0, n)
	for i, s := range a {
		if i > 0 {
			b = append(b, sep...)
		}
		b = append(b, s...)
	}
	return string(b)
}

// ensure BypassReceiver implements Receiver
var _ Receiver = (*BypassReceiver)(nil)

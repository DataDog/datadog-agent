// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"context"
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/lookbacksender"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/ringbuffer"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

// Retention owns the metric lookback buffer and creates shadow sender managers
// that write into it. It is intentionally outside pkg/aggregator so binaries
// that do not enable lookback do not link the concrete buffer/sender code.
type Retention struct {
	defaultHostname string
	buffer          *ringbuffer.Buffer
}

// NewRetentionFromConfig creates the metric lookback retention backend from
// Agent configuration. It returns nil when metric lookback is disabled.
func NewRetentionFromConfig(cfg model.Reader, defaultHostname string) *Retention {
	if cfg == nil || !cfg.GetBool("metric_lookback.enabled") {
		return nil
	}
	return NewRetention(defaultHostname, ringbuffer.Options{
		Capacity:   cfg.GetInt("metric_lookback.capacity"),
		ShardCount: cfg.GetInt("metric_lookback.shard_count"),
	})
}

// NewRetention creates a metric lookback retention backend with the provided
// ring buffer options.
func NewRetention(defaultHostname string, opts ringbuffer.Options) *Retention {
	return &Retention{
		defaultHostname: defaultHostname,
		buffer:          ringbuffer.New(opts),
	}
}

// NewSenderManager returns a shadow-check sender manager backed by the shared
// retention buffer. The context is scoped to the shadow check using the returned
// manager so in-flight writes can observe cancellation.
func (r *Retention) NewSenderManager(ctx context.Context) sender.SenderManager {
	if r == nil {
		return nil
	}
	return lookbacksender.NewSenderManager(ctx, r.defaultHostname, r.buffer, nil)
}

// Dump sends every sample currently retained in the metric lookback ring buffer
// through the provided serializer as a one-shot iterable series payload. It
// returns the number of series sent. The dump is non-destructive.
func (r *Retention) Dump(metricSerializer serializer.MetricSerializer) (int, error) {
	return r.DumpRange(metricSerializer, time.Time{}, time.Time{})
}

// DumpRange sends samples whose original timestamps fall in the inclusive
// [from, to] window through the provided serializer as a one-shot iterable
// series payload. A zero from or to leaves that side of the window unbounded. It
// returns the number of series sent. The dump is non-destructive.
func (r *Retention) DumpRange(metricSerializer serializer.MetricSerializer, from, to time.Time) (int, error) {
	if r == nil || r.buffer == nil {
		return 0, errors.New("metric lookback is disabled")
	}
	if metricSerializer == nil {
		return 0, errors.New("serializer is not available")
	}

	source := r.buffer.SerieSourceBetween(from, to)
	count := int(source.Count())
	if count == 0 {
		return 0, nil
	}
	if err := metricSerializer.SendIterableSeries(source); err != nil {
		return 0, err
	}
	return count, nil
}

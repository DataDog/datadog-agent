// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/lookbacksender"
	corelookback "github.com/DataDog/datadog-agent/pkg/metriclookback"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/ringbuffer"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// NewSenderManager returns a shadow-check sender manager backed by the shared
// retention ring. It is the collector/check integration point for checks that
// should populate metric lookback without emitting their samples through the
// normal check sender path.
func NewSenderManager(ctx context.Context, defaultHostname string, retention *corelookback.Retention) sender.SenderManager {
	if retention == nil {
		return nil
	}
	return lookbacksender.NewSenderManager(ctx, defaultHostname, shadowWriter{retention: retention}, nil)
}

type shadowWriter struct {
	retention *corelookback.Retention
}

// Append stores scalar samples emitted by a lookback shadow check. It satisfies
// lookbacksender.Writer and records the shadow check ID as retention source
// provenance without adding tags or changing the metric identity that egress
// forwards later.
func (w shadowWriter) Append(ctx context.Context, checkID checkid.ID, samples []metrics.MetricSample) error {
	if w.retention == nil {
		return nil
	}
	err := w.retention.AppendSamples(ctx, ringbuffer.Source{Kind: ringbuffer.SourceCheckShadow, ID: string(checkID)}, samples)
	if err != nil {
		return err
	}
	w.retention.ObserveSamples(samples)
	return nil
}

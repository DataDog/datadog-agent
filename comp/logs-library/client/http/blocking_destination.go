// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package http

import (
	"context"
	"errors"

	secretsnoopimpl "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// BlockingDestination sends payloads to an endpoint in the calling goroutine and returns the
// outcome. Unlike Destination, it has no send loop, buffer or retries: the caller owns retries, and
// only errors that wrap a client.RetryableError can succeed when retried. Errors caused by the
// intake response carry its status.
type BlockingDestination struct {
	destination *Destination
}

// NewBlockingDestination returns a BlockingDestination that sends payloads of contentType to endpoint.
func NewBlockingDestination(endpoint config.Endpoint, contentType string, cfg pkgconfigmodel.Reader) *BlockingDestination {
	return &BlockingDestination{
		// Send passes its own context, so the destinations context is never started.
		destination: newDestination(endpoint, contentType, client.NewDestinationsContext(), NoTimeoutOverride, false,
			client.NewNoopDestinationMetadata(), cfg, 1, 1, metrics.NewNoopPipelineMonitor(""), "", secretsnoopimpl.NewComponent().Comp),
	}
}

// Send sends payload, bound to ctx, and returns once the intake answered or ctx is done.
func (d *BlockingDestination) Send(ctx context.Context, payload *message.Payload) error {
	err := d.destination.unconditionalSendWithContext(ctx, payload)
	if err != nil {
		metrics.DestinationErrors.Add(1)
		metrics.TlmDestinationErrors.Inc()
		if errors.Is(err, errClient) {
			metrics.DestinationLogsDropped.Add(d.destination.host, payload.Count())
			metrics.TlmLogsDropped.Add(float64(payload.Count()), d.destination.host)
		}
		return err
	}
	metrics.LogsSent.Add(payload.Count())
	metrics.TlmLogsSent.Add(float64(payload.Count()))
	return nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/capture"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/output"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/report"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/safety"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
)

// AgentDelivery uses the same common pipeline for Windows and macOS samples.
// It never imports or initializes a live native collector.
type AgentDelivery struct{ Pipeline *output.Pipeline }

func (a AgentDelivery) Send(ctx context.Context, at time.Time, stream schema.Stream, samples []*telemetry.Sample) error {
	if len(samples) == 0 {
		return errors.New("empty required collection cycle")
	}
	switch stream {
	case schema.Processes, schema.Connections:
		messages := make([]model.MessageBody, 0, len(samples))
		var host, name string
		for _, s := range samples {
			if stream == schema.Processes {
				host = s.Processes.HostName
				name = checks.ProcessCheckName
				messages = append(messages, s.Processes)
			} else {
				host = s.Connections.HostName
				name = checks.ConnectionsCheckName
				messages = append(messages, s.Connections)
			}
		}
		if err := a.Pipeline.Submitter.SubmitForHost(ctx, at, name, host, &types.Payload{Message: messages}); err != nil {
			return err
		}
	case schema.Metrics:
		var series []*metrics.Serie
		for _, s := range samples {
			series = append(series, s.Metrics...)
		}
		if err := a.Pipeline.Serializer.SendIterableSeries(capture.NewSeriesSource(series)); err != nil {
			return err
		}
	case schema.HostMetadata:
		for _, s := range samples {
			if err := a.Pipeline.Serializer.SendHostMetadata(s.HostMetadata); err != nil {
				return err
			}
		}
	case schema.Software:
		for _, s := range samples {
			body, err := s.Software.MarshalJSON()
			if err != nil {
				return err
			}
			if err := a.Pipeline.Event(ctx, eventplatform.EventTypeSoftwareInventory, body, at); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported replay stream %s", stream)
	}
	// Every payload in this cycle has now been registered. Concurrent cycles can
	// extend the drain, but cannot let this cycle finish before its own acceptance.
	return a.Pipeline.Wait(ctx)
}
func (a AgentDelivery) NetworkMetrics(ctx context.Context, series []*metrics.Serie) error {
	if len(series) == 0 {
		return errors.New("access-point cycle contains no metrics")
	}
	if err := a.Pipeline.Serializer.SendIterableSeries(capture.NewSeriesSource(series)); err != nil {
		return err
	}
	return a.Pipeline.Wait(ctx)
}
func (a AgentDelivery) NetworkMetadata(ctx context.Context, payloads []metadata.NetworkDevicesMetadata) error {
	if len(payloads) == 0 {
		return errors.New("access-point cycle contains no metadata")
	}
	for _, payload := range payloads {
		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if err := a.Pipeline.Event(ctx, eventplatform.EventTypeNetworkDevicesMetadata, body, time.Unix(payload.CollectTimestamp, 0)); err != nil {
			return err
		}
	}
	return a.Pipeline.Wait(ctx)
}
func (a AgentDelivery) Wait(ctx context.Context) error { return a.Pipeline.Wait(ctx) }

// Execution limits bound concurrency, memory, and retry time without changing
// the scenario's fleet size or scheduled collection cycles.
const (
	replayWorkers       = 8
	replayQueueCapacity = 128
	deliveryGrace       = 5 * time.Minute
)

type ExecutionOptions struct {
	ReportPath, APIKey string
	Destinations       map[safety.Destination][]string
}

// Execute is the wall-clock staging lifecycle. Reserve the report before any
// forwarder starts, and retain failure accounting even when delivery is partial.
func Execute(ctx context.Context, request Request, options ExecutionOptions) error {
	prepared, err := prepare(request)
	if err != nil {
		return err
	}
	if options.APIKey == "" {
		return errors.New("set DD_API_KEY to the target staging organization's API key")
	}
	file, err := os.OpenFile(options.ReportPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("reserve local run report: %w", err)
	}
	defer file.Close()
	write := func(r *report.Report) error {
		data, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		if _, err := file.Seek(0, 0); err != nil {
			return err
		}
		if err := file.Truncate(0); err != nil {
			return err
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			return err
		}
		return file.Sync()
	}
	prepared.report.Status = "running"
	if err := write(prepared.report); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	pipeline, err := output.New(ctx, options.Destinations, options.APIKey, nil, output.Options{QueueCapacity: replayQueueCapacity})
	if err != nil {
		prepared.report.Status = "failed"
		prepared.report.End = time.Now()
		prepared.report.Errors = append(prepared.report.Errors, err.Error())
		if reportErr := write(prepared.report); reportErr != nil {
			return fmt.Errorf("delivery startup failed (%v), report write failed: %w", err, reportErr)
		}
		return err
	}
	defer pipeline.Close()
	// Validation and forwarder startup must not consume the scenario's duration.
	// The CLI supplies provisional metadata; execution establishes the real start.
	request.Plan.Start = time.Now().UTC()
	prepared.report.Start = request.Plan.Start
	if err := write(prepared.report); err != nil {
		return err
	}
	deadline := request.Plan.Start.Add(prepared.duration).Add(deliveryGrace)
	ctx, cancelDeadline := context.WithDeadline(ctx, deadline)
	defer cancelDeadline()
	result, runErr := prepared.run(ctx, Options{Workers: replayWorkers, QueueCapacity: replayQueueCapacity, Clock: WallClock{}, Delivery: AgentDelivery{Pipeline: pipeline}})
	if err := write(result); err != nil {
		return fmt.Errorf("write final local report (run error: %v): %w", runErr, err)
	}
	return runErr
}

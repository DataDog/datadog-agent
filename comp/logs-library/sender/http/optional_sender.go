// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package http

import (
	"sync"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

var (
	tlmOptionalPayloadsDropped = telemetryimpl.GetCompatComponent().NewCounter("logs_routing", "optional_payloads_dropped", []string{"endpoint"}, "Optional routed payloads dropped due to queue saturation")
	tlmOptionalMessagesDropped = telemetryimpl.GetCompatComponent().NewCounter("logs_routing", "optional_messages_dropped", []string{"endpoint"}, "Optional routed messages dropped due to queue saturation")
)

// OptionalSender sends best-effort payloads to a single unreliable HTTP endpoint.
// It never retries, never blocks on queue saturation, and never emits auditor outcomes.
type OptionalSender struct {
	config          pkgconfigmodel.Reader
	endpointID      string
	input           chan *message.Payload
	noopSink        chan *message.Payload
	noopSinkDone    chan struct{}
	dest            client.Destination
	destSender      *sender.DestinationSender
	bufferSize      int
	pipelineMonitor metrics.PipelineMonitor
	done            chan struct{}
	stopOnce        sync.Once
	started         bool
	mu              sync.Mutex
}

// NewOptionalHTTPSender creates a bounded, non-blocking HTTP sender for an unreliable endpoint.
func NewOptionalHTTPSender(
	cfg pkgconfigmodel.Reader,
	endpointID string,
	endpoint config.Endpoint,
	destinationsCtx *client.DestinationsContext,
	componentName string,
	contentType string,
	bufferSize int,
	maxConcurrency int,
	secretsComp secrets.Component,
	pipelineMonitor metrics.PipelineMonitor,
) *OptionalSender {
	if bufferSize < 1 {
		bufferSize = 1
	}
	if maxConcurrency < 1 {
		maxConcurrency = 1
	}
	destMeta := client.NewDestinationMetadata(componentName, "optional", "unreliable", endpointID, "")
	dest := http.NewDestination(endpoint, contentType, destinationsCtx, false, destMeta, cfg, 1, maxConcurrency, pipelineMonitor, "optional-"+endpointID, secretsComp)
	return &OptionalSender{
		config:          cfg,
		endpointID:      endpointID,
		input:           make(chan *message.Payload, bufferSize),
		dest:            dest,
		bufferSize:      bufferSize,
		pipelineMonitor: pipelineMonitor,
		done:            make(chan struct{}),
	}
}

// TrySend attempts a non-blocking enqueue to the optional sender. If the queue
// is full, it records payload and message drop telemetry once and returns false.
func (s *OptionalSender) TrySend(payload *message.Payload) bool {
	if payload == nil {
		return true
	}
	select {
	case s.input <- payload:
		return true
	default:
		s.recordDrop(payload)
		return false
	}
}

// RecordInputDrop counts one optional copy lost before it entered a batch.
func (s *OptionalSender) RecordInputDrop() {
	tlmOptionalMessagesDropped.Inc(s.endpointID)
}

func (s *OptionalSender) recordDrop(payload *message.Payload) {
	tlmOptionalPayloadsDropped.Inc(s.endpointID)
	tlmOptionalMessagesDropped.Add(float64(payload.Count()), s.endpointID)
}

// PipelineMonitor returns the sender's pipeline monitor.
func (s *OptionalSender) PipelineMonitor() metrics.PipelineMonitor { return s.pipelineMonitor }

// Start starts the optional HTTP destination and non-blocking dispatcher.
func (s *OptionalSender) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return
	}
	s.started = true
	s.pipelineMonitor.Start()
	s.noopSink = make(chan *message.Payload, s.bufferSize)
	s.noopSinkDone = make(chan struct{})
	go func() {
		defer close(s.noopSinkDone)
		for range s.noopSink {
		}
	}()
	s.destSender = sender.NewDestinationSender(s.config, s.dest, s.noopSink, s.bufferSize)
	go s.run()
}

func (s *OptionalSender) run() {
	defer close(s.done)
	for payload := range s.input {
		if !s.destSender.NonBlockingSend(payload) {
			s.recordDrop(payload)
		}
	}
}

// Stop closes the input queue and stops the underlying unreliable destination.
func (s *OptionalSender) Stop() {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		started := s.started
		s.mu.Unlock()
		close(s.input)
		if !started {
			return
		}
		<-s.done
		s.destSender.Stop()
		close(s.noopSink)
		<-s.noopSinkDone
		s.pipelineMonitor.Stop()
	})
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package consumer

import (
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/proto/api"
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	smodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProcessConsumer is part of the event monitoring module of the system-probe. It receives
// events, batches them in the messages channel and serves the messages to the process-agent
// over GRPC when requested
type ProcessConsumer struct {
	api.EventMonitoringModuleServer
	messages        chan *api.ProcessEventMessage
	maxMessageBurst int
	expiredEvents   *atomic.Int64

	statsdClient statsd.ClientInterface
}

// NewProcessConsumer returns a new ProcessConsumer instance
func NewProcessConsumer(evm *eventmonitor.EventMonitor) (*ProcessConsumer, error) {
	p := &ProcessConsumer{
		messages:        make(chan *api.ProcessEventMessage, evm.Config.EventServerBurst*3),
		maxMessageBurst: evm.Config.EventServerBurst,
		expiredEvents:   atomic.NewInt64(0),
		statsdClient:    evm.StatsdClient,
	}

	api.RegisterEventMonitoringModuleServer(evm.GRPCServer, p)

	if err := evm.AddEventTypeHandler(smodel.ForkEventType, p); err != nil {
		return nil, err
	}
	if err := evm.AddEventTypeHandler(smodel.ExecEventType, p); err != nil {
		return nil, err
	}
	if err := evm.AddEventTypeHandler(smodel.ExitEventType, p); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *ProcessConsumer) Start() error {
	return nil
}

func (p *ProcessConsumer) Stop() {
}

// ID returns id for process monitor
func (p *ProcessConsumer) ID() string {
	return "PROCESS"
}

func (p *ProcessConsumer) SendStats() {
	if count := p.expiredEvents.Swap(0); count > 0 {
		if err := p.statsdClient.Count(metrics.MetricProcessEventsServerExpired, count, []string{}, 1.0); err != nil {
			log.Warnf("Error sending process consumer stats: %v", err)
		}
	}
}

// HandleEvent implement the event monitor EventHandler interface
func (p *ProcessConsumer) HandleEvent(event *model.ProcessEvent) {
	data, err := event.MarshalMsg(nil)
	if err != nil {
		log.Errorf("Failed to marshal Process Lifecycle Event: %v", err)
		return
	}

	m := &api.ProcessEventMessage{
		Data: data,
	}

	select {
	case p.messages <- m:
		break
	default:
		// The channel is full, expire the oldest event
		<-p.messages
		p.expiredEvents.Inc()
		// Try to send the event again
		select {
		case p.messages <- m:
			break
		default:
			// looks like the process msgs channel is full again, expire the current event
			p.expiredEvents.Inc()
			break
		}
		break
	}
}

// GetProcessEvents sends process events through a gRPC stream
func (p *ProcessConsumer) GetProcessEvents(params *api.GetProcessEventParams, stream api.EventMonitoringModule_GetProcessEventsServer) error {
	msgs := 0
	timeout := time.Duration(params.TimeoutSeconds) * time.Second

	for msgs < p.maxMessageBurst {
		select {
		case msg := <-p.messages:
			if err := stream.Send(msg); err != nil {
				return err
			}
			msgs++
		case <-time.After(timeout):
			return nil
		}
	}

	log.Debugf("Received process-events request from process-agent: sent %d events", msgs)

	return nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatformimpl contains the logic for the no-sender eventplatform component
package eventplatformimpl

import (
	"context"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/pipeline"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defined the eventplatform requirements
type Requires struct {
	compdef.In
	Config                config.Component
	EventPlatformReceiver eventplatformreceiver.Component
	Hostname              hostnameinterface.Component
	Lc                    compdef.Lifecycle
}

type noSender struct {
	purgeMx   sync.Mutex
	pipelines map[string]*pipeline.PassthroughPipeline
}

// SendEventPlatformEvent sends messages to the event platform intake.
// SendEventPlatformEvent will drop messages and return an error if the input channel is already full.
func (s *noSender) SendEventPlatformEvent(e *message.Message, eventType string) error {
	p, ok := s.pipelines[eventType]
	if !ok {
		return fmt.Errorf("unknown eventType=%s", eventType)
	}

	// Stream to console if debug mode is enabled
	p.EventPlatformReceiver.HandleMessage(e, []byte{}, eventType)

	select {
	case p.In <- e:
		return nil
	default:
		return fmt.Errorf("event platform forwarder pipeline channel is full for eventType=%s. Channel capacity is %d. consider increasing batch_max_concurrent_send", eventType, cap(p.In))
	}
}

// SendEventPlatformEventBlocking sends messages to the event platform intake.
// SendEventPlatformEventBlocking will block if the input channel is already full.
func (s *noSender) SendEventPlatformEventBlocking(e *message.Message, eventType string) error {
	p, ok := s.pipelines[eventType]
	if !ok {
		return fmt.Errorf("unknown eventType=%s", eventType)
	}

	// Stream to console if debug mode is enabled
	p.EventPlatformReceiver.HandleMessage(e, []byte{}, eventType)

	p.In <- e
	return nil
}

func purgeChan(in chan *message.Message) (result []*message.Message) {
	for {
		select {
		case m, isOpen := <-in:
			if !isOpen {
				return
			}
			result = append(result, m)
		default:
			return
		}
	}
}

// Purge clears out all pipeline channels, returning a map of eventType to list of messages in that were removed from each channel
func (s *noSender) Purge() map[string][]*message.Message {
	s.purgeMx.Lock()
	defer s.purgeMx.Unlock()
	result := make(map[string][]*message.Message)
	for eventType, p := range s.pipelines {
		res := purgeChan(p.In)
		result[eventType] = res
		if eventType == eventplatform.EventTypeDBMActivity || eventType == eventplatform.EventTypeDBMMetrics || eventType == eventplatform.EventTypeDBMSamples {
			log.Debugf("purged DBM channel %s: %d events", eventType, len(res))
		}
	}
	return result
}

// NewComponent creates a new EventPlatformForwarder
func NewComponent(reqs Requires) eventplatform.Component {
	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()
	pipelines := make(map[string]*pipeline.PassthroughPipeline)
	for i, desc := range pipeline.PassthroughPipelineDescs {
		p, err := pipeline.NewHTTPPassthroughPipeline(reqs.Config, reqs.EventPlatformReceiver, desc, destinationsCtx, i, false)
		if err != nil {
			log.Errorf("Failed to initialize event platform forwarder pipeline. eventType=%s, error=%s", desc.EventType, err.Error())
			continue
		}
		pipelines[desc.EventType] = p
	}

	reqs.Lc.Append(compdef.Hook{
		OnStop: func(context.Context) error {
			log.Debugf("shutting down event platform forwarder")
			destinationsCtx.Stop()
			log.Debugf("event platform forwarder shut down complete")
			return nil
		},
	})

	return &noSender{
		pipelines: pipelines,
	}
}

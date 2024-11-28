// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatformimpl contains the logic for forwarding events to the event platform
package eventplatformimpl

import (
	"context"
	"fmt"
	"sync"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/pipeline"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

type eventPlatformForwarder struct {
	purgeMx         sync.Mutex
	pipelines       map[string]*pipeline.PassthroughPipeline
	destinationsCtx *client.DestinationsContext
}

// SendEventPlatformEvent sends messages to the event platform intake.
// SendEventPlatformEvent will drop messages and return an error if the input channel is already full.
func (s *eventPlatformForwarder) SendEventPlatformEvent(e *message.Message, eventType string) error {
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

// Diagnose enumerates known epforwarder pipelines and endpoints to test each of them connectivity
func Diagnose() []diagnosis.Diagnosis {
	var diagnoses []diagnosis.Diagnosis

	for _, desc := range pipeline.PassthroughPipelineDescs {
		configKeys := config.NewLogsConfigKeys(desc.EndpointsConfigPrefix, pkgconfigsetup.Datadog())
		endpoints, err := config.BuildHTTPEndpointsWithConfig(pkgconfigsetup.Datadog(), configKeys, desc.HostnameEndpointPrefix, desc.IntakeTrackType, config.DefaultIntakeProtocol, config.DefaultIntakeOrigin)
		if err != nil {
			diagnoses = append(diagnoses, diagnosis.Diagnosis{
				Result:      diagnosis.DiagnosisFail,
				Name:        "Endpoints configuration",
				Diagnosis:   "Misconfiguration of agent endpoints",
				Remediation: "Please validate Agent configuration",
				RawError:    err.Error(),
			})
			continue
		}

		url, err := logshttp.CheckConnectivityDiagnose(endpoints.Main, pkgconfigsetup.Datadog())
		name := fmt.Sprintf("Connectivity to %s", url)
		if err == nil {
			diagnoses = append(diagnoses, diagnosis.Diagnosis{
				Result:    diagnosis.DiagnosisSuccess,
				Category:  desc.Category,
				Name:      name,
				Diagnosis: fmt.Sprintf("Connectivity to `%s` is Ok", url),
			})
		} else {
			diagnoses = append(diagnoses, diagnosis.Diagnosis{
				Result:      diagnosis.DiagnosisFail,
				Category:    desc.Category,
				Name:        name,
				Diagnosis:   fmt.Sprintf("Connection to `%s` failed", url),
				Remediation: "Please validate Agent configuration and firewall to access " + url,
				RawError:    err.Error(),
			})
		}
	}

	return diagnoses
}

// SendEventPlatformEventBlocking sends messages to the event platform intake.
// SendEventPlatformEventBlocking will block if the input channel is already full.
func (s *eventPlatformForwarder) SendEventPlatformEventBlocking(e *message.Message, eventType string) error {
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
func (s *eventPlatformForwarder) Purge() map[string][]*message.Message {
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

func (s *eventPlatformForwarder) start() {
	s.destinationsCtx.Start()
	for _, p := range s.pipelines {
		p.Start()
	}
}

func (s *eventPlatformForwarder) stop() {
	log.Debugf("shutting down event platform forwarder")
	stopper := startstop.NewParallelStopper()
	for _, p := range s.pipelines {
		stopper.Add(p)
	}
	stopper.Stop()
	// TODO: wait on stop and cancel context only after timeout like logs agent
	s.destinationsCtx.Stop()
	log.Debugf("event platform forwarder shut down complete")
}

func newEventPlatformForwarder(config model.Reader, eventPlatformReceiver eventplatformreceiver.Component) *eventPlatformForwarder {
	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()
	pipelines := make(map[string]*pipeline.PassthroughPipeline)
	for i, desc := range pipeline.PassthroughPipelineDescs {
		p, err := pipeline.NewHTTPPassthroughPipeline(config, eventPlatformReceiver, desc, destinationsCtx, i, true)
		if err != nil {
			log.Errorf("Failed to initialize event platform forwarder pipeline. eventType=%s, error=%s", desc.EventType, err.Error())
			continue
		}
		pipelines[desc.EventType] = p
	}
	return &eventPlatformForwarder{
		pipelines:       pipelines,
		destinationsCtx: destinationsCtx,
	}
}

// Requires defined the eventplatform requirements
type Requires struct {
	compdef.In
	Config                configcomp.Component
	Lc                    compdef.Lifecycle
	EventPlatformReceiver eventplatformreceiver.Component
	Hostname              hostnameinterface.Component
}

// NewComponent creates a new EventPlatformForwarder
func NewComponent(reqs Requires) eventplatform.Component {
	forwarder := newEventPlatformForwarder(reqs.Config, reqs.EventPlatformReceiver)

	reqs.Lc.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			forwarder.start()
			return nil
		},
		OnStop: func(context.Context) error {
			forwarder.stop()
			return nil
		},
	})
	return forwarder
}

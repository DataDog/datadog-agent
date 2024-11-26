// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatformimpl contains the logic for forwarding events to the event platform
package eventplatformimpl

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

type defaultEventPlatformForwarder struct {
	purgeMx         sync.Mutex
	pipelines       map[string]*pipeline.PassthroughPipeline
	destinationsCtx *client.DestinationsContext
}

// SendEventPlatformEvent sends messages to the event platform intake.
// SendEventPlatformEvent will drop messages and return an error if the input channel is already full.
func (s *defaultEventPlatformForwarder) SendEventPlatformEvent(e *message.Message, eventType string) error {
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
func (s *defaultEventPlatformForwarder) SendEventPlatformEventBlocking(e *message.Message, eventType string) error {
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
func (s *defaultEventPlatformForwarder) Purge() map[string][]*message.Message {
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

func (s *defaultEventPlatformForwarder) start() {
	s.destinationsCtx.Start()
	for _, p := range s.pipelines {
		p.Start()
	}
}

func (s *defaultEventPlatformForwarder) stop() {
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

// newHTTPPassthroughPipeline creates a new HTTP-only event platform pipeline that sends messages directly to intake
// without any of the processing that exists in regular logs pipelines.
func newHTTPPassthroughPipeline(coreConfig model.Reader, eventPlatformReceiver eventplatformreceiver.Component, desc pipeline.PassthroughPipelineDesc, destinationsContext *client.DestinationsContext, pipelineID int) (p *pipeline.PassthroughPipeline, err error) {
	configKeys := config.NewLogsConfigKeys(desc.EndpointsConfigPrefix, pkgconfigsetup.Datadog())
	endpoints, err := config.BuildHTTPEndpointsWithConfig(pkgconfigsetup.Datadog(), configKeys, desc.HostnameEndpointPrefix, desc.IntakeTrackType, config.DefaultIntakeProtocol, config.DefaultIntakeOrigin)
	if err != nil {
		return nil, err
	}
	if !endpoints.UseHTTP {
		return nil, fmt.Errorf("endpoints must be http")
	}
	// epforwarder pipelines apply their own defaults on top of the hardcoded logs defaults
	if endpoints.BatchMaxConcurrentSend <= 0 {
		endpoints.BatchMaxConcurrentSend = desc.DefaultBatchMaxConcurrentSend
	}
	if endpoints.BatchMaxContentSize <= pkgconfigsetup.DefaultBatchMaxContentSize {
		endpoints.BatchMaxContentSize = desc.DefaultBatchMaxContentSize
	}
	if endpoints.BatchMaxSize <= pkgconfigsetup.DefaultBatchMaxSize {
		endpoints.BatchMaxSize = desc.DefaultBatchMaxSize
	}
	if endpoints.InputChanSize <= pkgconfigsetup.DefaultInputChanSize {
		endpoints.InputChanSize = desc.DefaultInputChanSize
	}

	pipelineMonitor := metrics.NewNoopPipelineMonitor(strconv.Itoa(pipelineID))

	reliable := []client.Destination{}
	for i, endpoint := range endpoints.GetReliableEndpoints() {
		destMeta := client.NewDestinationMetadata(desc.EventType, pipelineMonitor.ID(), "reliable", strconv.Itoa(i))
		reliable = append(reliable, logshttp.NewDestination(endpoint, desc.ContentType, destinationsContext, endpoints.BatchMaxConcurrentSend, true, destMeta, pkgconfigsetup.Datadog(), pipelineMonitor))
	}
	additionals := []client.Destination{}
	for i, endpoint := range endpoints.GetUnReliableEndpoints() {
		destMeta := client.NewDestinationMetadata(desc.EventType, pipelineMonitor.ID(), "unreliable", strconv.Itoa(i))
		additionals = append(additionals, logshttp.NewDestination(endpoint, desc.ContentType, destinationsContext, endpoints.BatchMaxConcurrentSend, false, destMeta, pkgconfigsetup.Datadog(), pipelineMonitor))
	}
	destinations := client.NewDestinations(reliable, additionals)
	inputChan := make(chan *message.Message, endpoints.InputChanSize)
	senderInput := make(chan *message.Payload, 1) // Only buffer 1 message since payloads can be large

	encoder := sender.IdentityContentType
	if endpoints.Main.UseCompression {
		encoder = sender.NewGzipContentEncoding(endpoints.Main.CompressionLevel)
	}

	var strategy sender.Strategy
	if desc.ContentType == logshttp.ProtobufContentType {
		strategy = sender.NewStreamStrategy(inputChan, senderInput, encoder)
	} else {
		strategy = sender.NewBatchStrategy(inputChan,
			senderInput,
			make(chan struct{}),
			false,
			nil,
			sender.ArraySerializer,
			endpoints.BatchWait,
			endpoints.BatchMaxSize,
			endpoints.BatchMaxContentSize,
			desc.EventType,
			encoder,
			pipelineMonitor)
	}

	a := auditor.NewNullAuditor()
	log.Debugf("Initialized event platform forwarder pipeline. eventType=%s mainHosts=%s additionalHosts=%s batch_max_concurrent_send=%d batch_max_content_size=%d batch_max_size=%d, input_chan_size=%d",
		desc.EventType, joinHosts(endpoints.GetReliableEndpoints()), joinHosts(endpoints.GetUnReliableEndpoints()), endpoints.BatchMaxConcurrentSend, endpoints.BatchMaxContentSize, endpoints.BatchMaxSize, endpoints.InputChanSize)
	return &pipeline.PassthroughPipeline{
		Sender:                sender.NewSender(coreConfig, senderInput, a.Channel(), destinations, 10, nil, nil, pipelineMonitor),
		Strategy:              strategy,
		In:                    inputChan,
		Auditor:               a,
		EventPlatformReceiver: eventPlatformReceiver,
	}, nil
}

func joinHosts(endpoints []config.Endpoint) string {
	var additionalHosts []string
	for _, e := range endpoints {
		additionalHosts = append(additionalHosts, e.Host)
	}
	return strings.Join(additionalHosts, ",")
}

func newDefaultEventPlatformForwarder(config model.Reader, eventPlatformReceiver eventplatformreceiver.Component) *defaultEventPlatformForwarder {
	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()
	pipelines := make(map[string]*pipeline.PassthroughPipeline)
	for i, desc := range pipeline.PassthroughPipelineDescs {
		p, err := newHTTPPassthroughPipeline(config, eventPlatformReceiver, desc, destinationsCtx, i)
		if err != nil {
			log.Errorf("Failed to initialize event platform forwarder pipeline. eventType=%s, error=%s", desc.EventType, err.Error())
			continue
		}
		pipelines[desc.EventType] = p
	}
	return &defaultEventPlatformForwarder{
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
	forwarder := newDefaultEventPlatformForwarder(reqs.Config, reqs.EventPlatformReceiver)

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

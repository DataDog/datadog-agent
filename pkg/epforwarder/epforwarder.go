// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package epforwarder

import (
	"fmt"
	"strings"
	"sync"

	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=epforwarder_mockgen.go

const (
	eventTypeDBMSamples  = "dbm-samples"
	eventTypeDBMMetrics  = "dbm-metrics"
	eventTypeDBMActivity = "dbm-activity"

	// EventTypeNetworkDevicesMetadata is the event type for network devices metadata
	EventTypeNetworkDevicesMetadata = "network-devices-metadata"

	// EventTypeSnmpTraps is the event type for snmp traps
	EventTypeSnmpTraps = "network-devices-snmp-traps"

	// EventTypeNetworkDevicesNetFlow is the event type for network devices NetFlow data
	EventTypeNetworkDevicesNetFlow = "network-devices-netflow"

	EventTypeContainerLifecycle = "container-lifecycle"
	EventTypeContainerImages    = "container-images"
	EventTypeContainerSBOM      = "container-sbom"
)

var passthroughPipelineDescs = []passthroughPipelineDesc{
	{
		eventType:              eventTypeDBMSamples,
		contentType:            http.JSONContentType,
		endpointsConfigPrefix:  "database_monitoring.samples.",
		hostnameEndpointPrefix: "dbm-metrics-intake.",
		intakeTrackType:        "databasequery",
		// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
		defaultBatchMaxConcurrentSend: 10,
		defaultBatchMaxContentSize:    10e6,
		defaultBatchMaxSize:           pkgconfig.DefaultBatchMaxSize,
		defaultInputChanSize:          pkgconfig.DefaultInputChanSize,
	},
	{
		eventType:              eventTypeDBMMetrics,
		contentType:            http.JSONContentType,
		endpointsConfigPrefix:  "database_monitoring.metrics.",
		hostnameEndpointPrefix: "dbm-metrics-intake.",
		intakeTrackType:        "dbmmetrics",
		// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
		defaultBatchMaxConcurrentSend: 10,
		defaultBatchMaxContentSize:    20e6,
		defaultBatchMaxSize:           pkgconfig.DefaultBatchMaxSize,
		defaultInputChanSize:          pkgconfig.DefaultInputChanSize,
	},
	{
		eventType:              eventTypeDBMActivity,
		contentType:            http.JSONContentType,
		endpointsConfigPrefix:  "database_monitoring.activity.",
		hostnameEndpointPrefix: "dbm-metrics-intake.",
		intakeTrackType:        "dbmactivity",
		// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
		defaultBatchMaxConcurrentSend: 10,
		defaultBatchMaxContentSize:    20e6,
		defaultBatchMaxSize:           pkgconfig.DefaultBatchMaxSize,
		defaultInputChanSize:          pkgconfig.DefaultInputChanSize,
	},
	{
		eventType:                     EventTypeNetworkDevicesMetadata,
		contentType:                   http.JSONContentType,
		endpointsConfigPrefix:         "network_devices.metadata.",
		hostnameEndpointPrefix:        "ndm-intake.",
		intakeTrackType:               "ndm",
		defaultBatchMaxConcurrentSend: 10,
		defaultBatchMaxContentSize:    pkgconfig.DefaultBatchMaxContentSize,
		defaultBatchMaxSize:           pkgconfig.DefaultBatchMaxSize,
		defaultInputChanSize:          pkgconfig.DefaultInputChanSize,
	},
	{
		eventType:                     EventTypeSnmpTraps,
		contentType:                   http.JSONContentType,
		endpointsConfigPrefix:         "network_devices.snmp_traps.forwarder.",
		hostnameEndpointPrefix:        "snmp-traps-intake.",
		intakeTrackType:               "ndmtraps",
		defaultBatchMaxConcurrentSend: 10,
		defaultBatchMaxContentSize:    pkgconfig.DefaultBatchMaxContentSize,
		defaultBatchMaxSize:           pkgconfig.DefaultBatchMaxSize,
		defaultInputChanSize:          pkgconfig.DefaultInputChanSize,
	},
	{
		eventType:                     EventTypeNetworkDevicesNetFlow,
		contentType:                   http.JSONContentType,
		endpointsConfigPrefix:         "network_devices.netflow.forwarder.",
		hostnameEndpointPrefix:        "ndmflow-intake.",
		intakeTrackType:               "ndmflow",
		defaultBatchMaxConcurrentSend: 10,
		defaultBatchMaxContentSize:    pkgconfig.DefaultBatchMaxContentSize,

		// Each NetFlow flow is about 500 bytes
		// 10k BatchMaxSize is about 5Mo of content size
		defaultBatchMaxSize: 10000,

		// High input chan is needed to handle high number of flows being flushed by NetFlow Server every 10s
		// Customers might need to set `network_devices.forwarder.input_chan_size` to higher value if flows are dropped
		// due to input channel being full.
		// TODO: A possible better solution is to make SendEventPlatformEvent blocking when input chan is full and avoid
		//   dropping events. This can't be done right now due to SendEventPlatformEvent being called by
		//   aggregator loop, making SendEventPlatformEvent blocking might slow down other type of data handled
		//   by aggregator.
		defaultInputChanSize: 10000,
	},
	{
		eventType:                     EventTypeContainerLifecycle,
		contentType:                   http.ProtobufContentType,
		endpointsConfigPrefix:         "container_lifecycle.",
		hostnameEndpointPrefix:        "contlcycle-intake.",
		intakeTrackType:               "contlcycle",
		defaultBatchMaxConcurrentSend: 10,
		defaultBatchMaxContentSize:    pkgconfig.DefaultBatchMaxContentSize,
		defaultBatchMaxSize:           pkgconfig.DefaultBatchMaxSize,
		defaultInputChanSize:          pkgconfig.DefaultInputChanSize,
	},
	{
		eventType:                     EventTypeContainerImages,
		contentType:                   http.ProtobufContentType,
		endpointsConfigPrefix:         "container_image.",
		hostnameEndpointPrefix:        "contimage-intake.",
		intakeTrackType:               "contimage",
		defaultBatchMaxConcurrentSend: 10,
		defaultBatchMaxContentSize:    pkgconfig.DefaultBatchMaxContentSize,
		defaultBatchMaxSize:           pkgconfig.DefaultBatchMaxSize,
		defaultInputChanSize:          pkgconfig.DefaultInputChanSize,
	},
	{
		eventType:                     EventTypeContainerSBOM,
		contentType:                   http.ProtobufContentType,
		endpointsConfigPrefix:         "sbom.",
		hostnameEndpointPrefix:        "sbom-intake.",
		intakeTrackType:               "sbom",
		defaultBatchMaxConcurrentSend: 10,
		defaultBatchMaxContentSize:    pkgconfig.DefaultBatchMaxContentSize,
		defaultBatchMaxSize:           pkgconfig.DefaultBatchMaxSize,
		defaultInputChanSize:          pkgconfig.DefaultInputChanSize,
	},
}

// An EventPlatformForwarder forwards Messages to a destination based on their event type
type EventPlatformForwarder interface {
	SendEventPlatformEvent(e *message.Message, eventType string) error
	SendEventPlatformEventBlocking(e *message.Message, eventType string) error
	Purge() map[string][]*message.Message
	Start()
	Stop()
}

type defaultEventPlatformForwarder struct {
	purgeMx         sync.Mutex
	pipelines       map[string]*passthroughPipeline
	destinationsCtx *client.DestinationsContext
}

// SendEventPlatformEvent sends messages to the event platform intake.
// SendEventPlatformEvent will drop messages and return an error if the input channel is already full.
func (s *defaultEventPlatformForwarder) SendEventPlatformEvent(e *message.Message, eventType string) error {
	p, ok := s.pipelines[eventType]
	if !ok {
		return fmt.Errorf("unknown eventType=%s", eventType)
	}
	select {
	case p.in <- e:
		return nil
	default:
		return fmt.Errorf("event platform forwarder pipeline channel is full for eventType=%s. Channel capacity is %d. consider increasing batch_max_concurrent_send", eventType, cap(p.in))
	}
}

// SendEventPlatformEventBlocking sends messages to the event platform intake.
// SendEventPlatformEventBlocking will block if the input channel is already full.
func (s *defaultEventPlatformForwarder) SendEventPlatformEventBlocking(e *message.Message, eventType string) error {
	p, ok := s.pipelines[eventType]
	if !ok {
		return fmt.Errorf("unknown eventType=%s", eventType)
	}
	p.in <- e
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
		res := purgeChan(p.in)
		result[eventType] = res
		if eventType == eventTypeDBMActivity || eventType == eventTypeDBMMetrics || eventType == eventTypeDBMSamples {
			log.Debugf("purged DBM channel %s: %d events", eventType, len(res))
		}
	}
	return result
}

func (s *defaultEventPlatformForwarder) Start() {
	s.destinationsCtx.Start()
	for _, p := range s.pipelines {
		p.Start()
	}
}

func (s *defaultEventPlatformForwarder) Stop() {
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

type passthroughPipeline struct {
	sender   *sender.Sender
	strategy sender.Strategy
	in       chan *message.Message
	auditor  auditor.Auditor
}

type passthroughPipelineDesc struct {
	eventType   string
	contentType string
	// intakeTrackType is the track type to use for the v2 intake api. When blank, v1 is used instead.
	intakeTrackType               config.IntakeTrackType
	endpointsConfigPrefix         string
	hostnameEndpointPrefix        string
	defaultBatchMaxConcurrentSend int
	defaultBatchMaxContentSize    int
	defaultBatchMaxSize           int
	defaultInputChanSize          int
}

// newHTTPPassthroughPipeline creates a new HTTP-only event platform pipeline that sends messages directly to intake
// without any of the processing that exists in regular logs pipelines.
func newHTTPPassthroughPipeline(desc passthroughPipelineDesc, destinationsContext *client.DestinationsContext, pipelineID int) (p *passthroughPipeline, err error) {
	configKeys := config.NewLogsConfigKeys(desc.endpointsConfigPrefix, coreConfig.Datadog)
	endpoints, err := config.BuildHTTPEndpointsWithConfig(configKeys, desc.hostnameEndpointPrefix, desc.intakeTrackType, config.DefaultIntakeProtocol, config.DefaultIntakeOrigin)
	if err != nil {
		return nil, err
	}
	if !endpoints.UseHTTP {
		return nil, fmt.Errorf("endpoints must be http")
	}
	// epforwarder pipelines apply their own defaults on top of the hardcoded logs defaults
	if endpoints.BatchMaxConcurrentSend <= 0 {
		endpoints.BatchMaxConcurrentSend = desc.defaultBatchMaxConcurrentSend
	}
	if endpoints.BatchMaxContentSize <= pkgconfig.DefaultBatchMaxContentSize {
		endpoints.BatchMaxContentSize = desc.defaultBatchMaxContentSize
	}
	if endpoints.BatchMaxSize <= pkgconfig.DefaultBatchMaxSize {
		endpoints.BatchMaxSize = desc.defaultBatchMaxSize
	}
	if endpoints.InputChanSize <= pkgconfig.DefaultInputChanSize {
		endpoints.InputChanSize = desc.defaultInputChanSize
	}
	reliable := []client.Destination{}
	for i, endpoint := range endpoints.GetReliableEndpoints() {
		telemetryName := fmt.Sprintf("%s_%d_reliable_%d", desc.eventType, pipelineID, i)
		reliable = append(reliable, http.NewDestination(endpoint, desc.contentType, destinationsContext, endpoints.BatchMaxConcurrentSend, true, telemetryName))
	}
	additionals := []client.Destination{}
	for i, endpoint := range endpoints.GetUnReliableEndpoints() {
		telemetryName := fmt.Sprintf("%s_%d_unreliable_%d", desc.eventType, pipelineID, i)
		additionals = append(additionals, http.NewDestination(endpoint, desc.contentType, destinationsContext, endpoints.BatchMaxConcurrentSend, false, telemetryName))
	}
	destinations := client.NewDestinations(reliable, additionals)
	inputChan := make(chan *message.Message, endpoints.InputChanSize)
	senderInput := make(chan *message.Payload, 1) // Only buffer 1 message since payloads can be large

	encoder := sender.IdentityContentType
	if endpoints.Main.UseCompression {
		encoder = sender.NewGzipContentEncoding(endpoints.Main.CompressionLevel)
	}

	var strategy sender.Strategy
	if desc.contentType == http.ProtobufContentType {
		strategy = sender.NewStreamStrategy(inputChan, senderInput, encoder)
	} else {
		strategy = sender.NewBatchStrategy(inputChan,
			senderInput,
			make(chan struct{}),
			sender.ArraySerializer,
			endpoints.BatchWait,
			endpoints.BatchMaxSize,
			endpoints.BatchMaxContentSize,
			desc.eventType,
			encoder)
	}

	a := auditor.NewNullAuditor()
	log.Debugf("Initialized event platform forwarder pipeline. eventType=%s mainHosts=%s additionalHosts=%s batch_max_concurrent_send=%d batch_max_content_size=%d batch_max_size=%d, input_chan_size=%d",
		desc.eventType, joinHosts(endpoints.GetReliableEndpoints()), joinHosts(endpoints.GetUnReliableEndpoints()), endpoints.BatchMaxConcurrentSend, endpoints.BatchMaxContentSize, endpoints.BatchMaxSize, endpoints.InputChanSize)
	return &passthroughPipeline{
		sender:   sender.NewSender(senderInput, a.Channel(), destinations, 10),
		strategy: strategy,
		in:       inputChan,
		auditor:  a,
	}, nil
}

func (p *passthroughPipeline) Start() {
	p.auditor.Start()
	if p.strategy != nil {
		p.strategy.Start()
		p.sender.Start()
	}
}

func (p *passthroughPipeline) Stop() {
	p.strategy.Stop()
	p.sender.Stop()
	p.auditor.Stop()
}

func joinHosts(endpoints []config.Endpoint) string {
	var additionalHosts []string
	for _, e := range endpoints {
		additionalHosts = append(additionalHosts, e.Host)
	}
	return strings.Join(additionalHosts, ",")
}

func newDefaultEventPlatformForwarder() *defaultEventPlatformForwarder {
	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()
	pipelines := make(map[string]*passthroughPipeline)
	for i, desc := range passthroughPipelineDescs {
		p, err := newHTTPPassthroughPipeline(desc, destinationsCtx, i)
		if err != nil {
			log.Errorf("Failed to initialize event platform forwarder pipeline. eventType=%s, error=%s", desc.eventType, err.Error())
			continue
		}
		pipelines[desc.eventType] = p
	}
	return &defaultEventPlatformForwarder{
		pipelines:       pipelines,
		destinationsCtx: destinationsCtx,
	}
}

// NewEventPlatformForwarder creates a new EventPlatformForwarder
func NewEventPlatformForwarder() EventPlatformForwarder {
	return newDefaultEventPlatformForwarder()
}

// NewNoopEventPlatformForwarder returns the standard event platform forwarder with sending disabled, meaning events
// will build up in each pipeline channel without being forwarded to the intake
func NewNoopEventPlatformForwarder() EventPlatformForwarder {
	f := newDefaultEventPlatformForwarder()
	// remove the senders
	for _, p := range f.pipelines {
		p.strategy = nil
	}
	return f
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package epforwarder contains the logic for forwarding events to the event platform
package epforwarder

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	aggsender "github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=epforwarder_mockgen.go

const (
	eventTypeDBMSamples  = "dbm-samples"
	eventTypeDBMMetrics  = "dbm-metrics"
	eventTypeDBMActivity = "dbm-activity"
	eventTypeDBMMetadata = "dbm-metadata"

	// EventTypeNetworkDevicesMetadata is the event type for network devices metadata
	EventTypeNetworkDevicesMetadata = "network-devices-metadata"

	// EventTypeSnmpTraps is the event type for snmp traps
	EventTypeSnmpTraps = "network-devices-snmp-traps"

	// EventTypeNetworkDevicesNetFlow is the event type for network devices NetFlow data
	EventTypeNetworkDevicesNetFlow = "network-devices-netflow"

	// EventTypeContainerLifecycle represents a container lifecycle event
	EventTypeContainerLifecycle = "container-lifecycle"
	// EventTypeContainerImages represents a container images event
	EventTypeContainerImages = "container-images"
	// EventTypeContainerSBOM represents a container SBOM event
	EventTypeContainerSBOM = "container-sbom"
)

var passthroughPipelineDescs = []passthroughPipelineDesc{
	{
		eventType:              eventTypeDBMSamples,
		category:               "DBM",
		contentType:            logshttp.JSONContentType,
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
		category:               "DBM",
		contentType:            logshttp.JSONContentType,
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
		eventType:   eventTypeDBMMetadata,
		contentType: logshttp.JSONContentType,
		// set the endpoint config to "metrics" since metadata will hit the same endpoint
		// as metrics, so there is no need to add an extra config endpoint.
		// As a follow-on PR, we should clean this up to have a single config for each track type since
		// all of our data now flows through the same intake
		endpointsConfigPrefix:  "database_monitoring.metrics.",
		hostnameEndpointPrefix: "dbm-metrics-intake.",
		intakeTrackType:        "dbmmetadata",
		// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
		defaultBatchMaxConcurrentSend: 10,
		defaultBatchMaxContentSize:    20e6,
		defaultBatchMaxSize:           pkgconfig.DefaultBatchMaxSize,
		defaultInputChanSize:          pkgconfig.DefaultInputChanSize,
	},
	{
		eventType:              eventTypeDBMActivity,
		category:               "DBM",
		contentType:            logshttp.JSONContentType,
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
		category:                      "NDM",
		contentType:                   logshttp.JSONContentType,
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
		category:                      "NDM",
		contentType:                   logshttp.JSONContentType,
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
		category:                      "NDM",
		contentType:                   logshttp.JSONContentType,
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
		category:                      "Container",
		contentType:                   logshttp.ProtobufContentType,
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
		category:                      "Container",
		contentType:                   logshttp.ProtobufContentType,
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
		category:                      "SBOM",
		contentType:                   logshttp.ProtobufContentType,
		endpointsConfigPrefix:         "sbom.",
		hostnameEndpointPrefix:        "sbom-intake.",
		intakeTrackType:               "sbom",
		defaultBatchMaxConcurrentSend: 10,
		defaultBatchMaxContentSize:    pkgconfig.DefaultBatchMaxContentSize,
		defaultBatchMaxSize:           pkgconfig.DefaultBatchMaxSize,
		defaultInputChanSize:          pkgconfig.DefaultInputChanSize,
	},
}

var globalReceiver *diagnostic.BufferedMessageReceiver

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
	panic("not called")
}

func init() {
	diagnosis.Register("connectivity-datadog-event-platform", diagnose)
}

// Enumerate known epforwarder pipelines and endpoints to test each of them connectivity
func diagnose(_ diagnosis.Config, _ aggsender.DiagnoseSenderManager) []diagnosis.Diagnosis {
	panic("not called")
}

// SendEventPlatformEventBlocking sends messages to the event platform intake.
// SendEventPlatformEventBlocking will block if the input channel is already full.
func (s *defaultEventPlatformForwarder) SendEventPlatformEventBlocking(e *message.Message, eventType string) error {
	panic("not called")
}

func purgeChan(in chan *message.Message) (result []*message.Message) {
	panic("not called")
}

// Purge clears out all pipeline channels, returning a map of eventType to list of messages in that were removed from each channel
func (s *defaultEventPlatformForwarder) Purge() map[string][]*message.Message {
	panic("not called")
}

func (s *defaultEventPlatformForwarder) Start() {
	panic("not called")
}

func (s *defaultEventPlatformForwarder) Stop() {
	panic("not called")
}

type passthroughPipeline struct {
	sender                    *sender.Sender
	strategy                  sender.Strategy
	in                        chan *message.Message
	auditor                   auditor.Auditor
	diagnosticMessageReceiver *diagnostic.BufferedMessageReceiver
}

type passthroughPipelineDesc struct {
	eventType   string
	category    string
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
	panic("not called")
}

func (p *passthroughPipeline) Start() {
	panic("not called")
}

func (p *passthroughPipeline) Stop() {
	panic("not called")
}

func joinHosts(endpoints []config.Endpoint) string {
	panic("not called")
}

func newDefaultEventPlatformForwarder() *defaultEventPlatformForwarder {
	panic("not called")
}

// NewEventPlatformForwarder creates a new EventPlatformForwarder
func NewEventPlatformForwarder() EventPlatformForwarder {
	panic("not called")
}

// NewNoopEventPlatformForwarder returns the standard event platform forwarder with sending disabled, meaning events
// will build up in each pipeline channel without being forwarded to the intake
func NewNoopEventPlatformForwarder() EventPlatformForwarder {
	panic("not called")
}

// GetGlobalReceiver initializes and returns the global receiver for the epforwarder package
func GetGlobalReceiver() *diagnostic.BufferedMessageReceiver {
	panic("not called")
}

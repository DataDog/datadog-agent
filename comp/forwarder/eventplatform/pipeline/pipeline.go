// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pipeline contains the definition of the pipelines used by the event platform.
package pipeline

import (
	"fmt"
	"strconv"
	"strings"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
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
)

// PassthroughPipelineDescs returns the list of passthrough pipelines
var PassthroughPipelineDescs = []PassthroughPipelineDesc{
	{
		EventType:              eventplatform.EventTypeDBMSamples,
		Category:               "DBM",
		ContentType:            logshttp.JSONContentType,
		EndpointsConfigPrefix:  "database_monitoring.samples.",
		HostnameEndpointPrefix: "dbm-metrics-intake.",
		IntakeTrackType:        "databasequery",
		// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
		DefaultBatchMaxConcurrentSend: 10,
		DefaultBatchMaxContentSize:    10e6,
		DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
		DefaultInputChanSize: 500,
	},
	{
		EventType:              eventplatform.EventTypeDBMMetrics,
		Category:               "DBM",
		ContentType:            logshttp.JSONContentType,
		EndpointsConfigPrefix:  "database_monitoring.metrics.",
		HostnameEndpointPrefix: "dbm-metrics-intake.",
		IntakeTrackType:        "dbmmetrics",
		// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
		DefaultBatchMaxConcurrentSend: 10,
		DefaultBatchMaxContentSize:    20e6,
		DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
		DefaultInputChanSize: 500,
	},
	{
		EventType:   eventplatform.EventTypeDBMMetadata,
		ContentType: logshttp.JSONContentType,
		// set the endpoint config to "metrics" since metadata will hit the same endpoint
		// as metrics, so there is no need to add an extra config endpoint.
		// As a follow-on PR, we should clean this up to have a single config for each track type since
		// all of our data now flows through the same intake
		EndpointsConfigPrefix:  "database_monitoring.metrics.",
		HostnameEndpointPrefix: "dbm-metrics-intake.",
		IntakeTrackType:        "dbmmetadata",
		// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
		DefaultBatchMaxConcurrentSend: 10,
		DefaultBatchMaxContentSize:    20e6,
		DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
		DefaultInputChanSize: 500,
	},
	{
		EventType:              eventplatform.EventTypeDBMActivity,
		Category:               "DBM",
		ContentType:            logshttp.JSONContentType,
		EndpointsConfigPrefix:  "database_monitoring.activity.",
		HostnameEndpointPrefix: "dbm-metrics-intake.",
		IntakeTrackType:        "dbmactivity",
		// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
		DefaultBatchMaxConcurrentSend: 10,
		DefaultBatchMaxContentSize:    20e6,
		DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
		DefaultInputChanSize: 500,
	},
	{
		EventType:                     eventplatform.EventTypeNetworkDevicesMetadata,
		Category:                      "NDM",
		ContentType:                   logshttp.JSONContentType,
		EndpointsConfigPrefix:         "network_devices.metadata.",
		HostnameEndpointPrefix:        "ndm-intake.",
		IntakeTrackType:               "ndm",
		DefaultBatchMaxConcurrentSend: 10,
		DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
		DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		DefaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
	},
	{
		EventType:                     eventplatform.EventTypeSnmpTraps,
		Category:                      "NDM",
		ContentType:                   logshttp.JSONContentType,
		EndpointsConfigPrefix:         "network_devices.snmp_traps.forwarder.",
		HostnameEndpointPrefix:        "snmp-traps-intake.",
		IntakeTrackType:               "ndmtraps",
		DefaultBatchMaxConcurrentSend: 10,
		DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
		DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		DefaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
	},
	{
		EventType:                     eventplatform.EventTypeNetworkDevicesNetFlow,
		Category:                      "NDM",
		ContentType:                   logshttp.JSONContentType,
		EndpointsConfigPrefix:         "network_devices.netflow.forwarder.",
		HostnameEndpointPrefix:        "ndmflow-intake.",
		IntakeTrackType:               "ndmflow",
		DefaultBatchMaxConcurrentSend: 10,
		DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,

		// Each NetFlow flow is about 500 bytes
		// 10k BatchMaxSize is about 5Mo of content size
		DefaultBatchMaxSize: 10000,

		// High input chan is needed to handle high number of flows being flushed by NetFlow Server every 10s
		// Customers might need to set `network_devices.forwarder.input_chan_size` to higher value if flows are dropped
		// due to input channel being full.
		// TODO: A possible better solution is to make SendEventPlatformEvent blocking when input chan is full and avoid
		//   dropping events. This can't be done right now due to SendEventPlatformEvent being called by
		//   aggregator loop, making SendEventPlatformEvent blocking might slow down other type of data handled
		//   by aggregator.
		DefaultInputChanSize: 10000,
	},
	{
		EventType:                     eventplatform.EventTypeNetworkPath,
		Category:                      "Network Path",
		ContentType:                   logshttp.JSONContentType,
		EndpointsConfigPrefix:         "network_path.forwarder.",
		HostnameEndpointPrefix:        "netpath-intake.",
		IntakeTrackType:               "netpath",
		DefaultBatchMaxConcurrentSend: 10,
		DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
		DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		DefaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
	},
	{
		EventType:                     eventplatform.EventTypeContainerLifecycle,
		Category:                      "Container",
		ContentType:                   logshttp.ProtobufContentType,
		EndpointsConfigPrefix:         "container_lifecycle.",
		HostnameEndpointPrefix:        "contlcycle-intake.",
		IntakeTrackType:               "contlcycle",
		DefaultBatchMaxConcurrentSend: 10,
		DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
		DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		DefaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
	},
	{
		EventType:                     eventplatform.EventTypeContainerImages,
		Category:                      "Container",
		ContentType:                   logshttp.ProtobufContentType,
		EndpointsConfigPrefix:         "container_image.",
		HostnameEndpointPrefix:        "contimage-intake.",
		IntakeTrackType:               "contimage",
		DefaultBatchMaxConcurrentSend: 10,
		DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
		DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		DefaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
	},
	{
		EventType:                     eventplatform.EventTypeContainerSBOM,
		Category:                      "SBOM",
		ContentType:                   logshttp.ProtobufContentType,
		EndpointsConfigPrefix:         "sbom.",
		HostnameEndpointPrefix:        "sbom-intake.",
		IntakeTrackType:               "sbom",
		DefaultBatchMaxConcurrentSend: 10,
		DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
		DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		DefaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
	},
	{
		EventType:                     eventplatform.EventTypeServiceDiscovery,
		Category:                      "Service Discovery",
		ContentType:                   logshttp.JSONContentType,
		EndpointsConfigPrefix:         "service_discovery.forwarder.",
		HostnameEndpointPrefix:        "instrumentation-telemetry-intake.",
		IntakeTrackType:               "apmtelemetry",
		DefaultBatchMaxConcurrentSend: 10,
		DefaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
		DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		DefaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
	},
}

// PassthroughPipeline is a pipeline that forwards logs to the event platform
type PassthroughPipeline struct {
	Sender                *sender.Sender
	Strategy              sender.Strategy
	In                    chan *message.Message
	Auditor               auditor.Auditor
	EventPlatformReceiver eventplatformreceiver.Component
}

// PassthroughPipelineDesc stores pipeline information
type PassthroughPipelineDesc struct {
	EventType   string
	Category    string
	ContentType string
	// intakeTrackType is the track type to use for the v2 intake api. When blank, v1 is used instead.
	IntakeTrackType               config.IntakeTrackType
	EndpointsConfigPrefix         string
	HostnameEndpointPrefix        string
	DefaultBatchMaxConcurrentSend int
	DefaultBatchMaxContentSize    int
	DefaultBatchMaxSize           int
	DefaultInputChanSize          int
}

// Start start the PassthroughPipeline
func (p *PassthroughPipeline) Start() {
	p.Strategy.Start()
	p.Sender.Start()
}

// Stop stops the PassthroughPipeline
func (p *PassthroughPipeline) Stop() {
	p.Strategy.Stop()
	p.Sender.Stop()
}

// NewHTTPPassthroughPipeline creates a new HTTP-only event platform pipeline that sends messages directly to intake
// without any of the processing that exists in regular logs pipelines.
func NewHTTPPassthroughPipeline(coreConfig model.Reader, eventPlatformReceiver eventplatformreceiver.Component, desc PassthroughPipelineDesc, destinationsContext *client.DestinationsContext, pipelineID int, enabledSender bool) (p *PassthroughPipeline, err error) {
	configKeys := config.NewLogsConfigKeys(desc.EndpointsConfigPrefix, coreConfig)
	endpoints, err := config.BuildHTTPEndpointsWithConfig(coreConfig, configKeys, desc.HostnameEndpointPrefix, desc.IntakeTrackType, config.DefaultIntakeProtocol, config.DefaultIntakeOrigin)
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

	pipeline := &PassthroughPipeline{
		In:                    inputChan,
		EventPlatformReceiver: eventPlatformReceiver,
	}

	if enabledSender {
		pipeline.Sender = sender.NewSender(coreConfig, senderInput, auditor.NewNullAuditor().Channel(), destinations, 10, nil, nil, pipelineMonitor)

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
		pipeline.Strategy = strategy
	}

	log.Debugf("Initialized event platform forwarder pipeline. eventType=%s mainHosts=%s additionalHosts=%s batch_max_concurrent_send=%d batch_max_content_size=%d batch_max_size=%d, input_chan_size=%d",
		desc.EventType, joinHosts(endpoints.GetReliableEndpoints()), joinHosts(endpoints.GetUnReliableEndpoints()), endpoints.BatchMaxConcurrentSend, endpoints.BatchMaxContentSize, endpoints.BatchMaxSize, endpoints.InputChanSize)

	return pipeline, nil

}

func joinHosts(endpoints []config.Endpoint) string {
	var additionalHosts []string
	for _, e := range endpoints {
		additionalHosts = append(additionalHosts, e.Host)
	}
	return strings.Join(additionalHosts, ",")
}

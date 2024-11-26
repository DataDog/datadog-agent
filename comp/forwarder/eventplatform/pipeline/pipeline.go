// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pipeline contains the definition of the pipelines used by the event platform.
package pipeline

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
)

// PassthroughPipelineDescs returns the list of passthrough pipelines
var PassthroughPipelineDescs = []PassthroughPipelineDesc{
	{
		EventType:              eventplatform.EventTypeDBMSamples,
		Category:               "DBM",
		ContentType:            "application/json",
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
		ContentType:            "application/json",
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
		ContentType: "application/json",
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
		ContentType:            "application/json",
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
		ContentType:                   "application/json",
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
		ContentType:                   "application/json",
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
		ContentType:                   "application/json",
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
		ContentType:                   "application/json",
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
		ContentType:                   "application/x-protobuf",
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
		ContentType:                   "application/x-protobuf",
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
		ContentType:                   "application/x-protobuf",
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
		ContentType:                   "application/json",
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
	p.Auditor.Start()
	if p.Strategy != nil {
		p.Strategy.Start()
		p.Sender.Start()
	}
}

// Stop stops the PassthroughPipeline
func (p *PassthroughPipeline) Stop() {
	if p.Strategy != nil {
		p.Strategy.Stop()
		p.Sender.Stop()
	}
	p.Auditor.Stop()
}

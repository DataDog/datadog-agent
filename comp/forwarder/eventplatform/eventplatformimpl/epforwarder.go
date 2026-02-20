// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatformimpl contains the logic for forwarding events to the event platform
package eventplatformimpl

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"go.uber.org/fx"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	httpsender "github.com/DataDog/datadog-agent/pkg/logs/sender/http"
	compressioncommon "github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=epforwarder_mockgen.go

// Module defines the fx options for this component.
func Module(params Params) fxutil.Module {
	return fxutil.Component(fx.Provide(newEventPlatformForwarder), fx.Supply(params))
}

const (
	eventTypeDBMSamples         = "dbm-samples"
	eventTypeDBMMetrics         = "dbm-metrics"
	eventTypeDBMActivity        = "dbm-activity"
	eventTypeDBMMetadata        = "dbm-metadata"
	eventTypeDBMHealth          = "dbm-health"
	eventTypeDataStreamsMessage = "data-streams-message"
)

func getPassthroughPipelines() []passthroughPipelineDesc {
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
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			defaultInputChanSize: 500,
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
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			defaultInputChanSize: 500,
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
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			defaultInputChanSize: 500,
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
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			defaultInputChanSize: 500,
		},
		{
			eventType:   eventTypeDBMHealth,
			contentType: logshttp.JSONContentType,
			// set the endpoint config to "metrics" since health will hit the same endpoint
			// as metrics, so there is no need to add an extra config endpoint.
			endpointsConfigPrefix:  "database_monitoring.metrics.",
			hostnameEndpointPrefix: "dbm-metrics-intake.",
			intakeTrackType:        "dbmhealth",
			// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    20e6,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			defaultInputChanSize: 500,
		}, {
			eventType:                     eventplatform.EventTypeNetworkDevicesMetadata,
			category:                      "NDM",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "network_devices.metadata.",
			hostnameEndpointPrefix:        "ndm-intake.",
			intakeTrackType:               "ndm",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
		{
			eventType:                     eventplatform.EventTypeSnmpTraps,
			category:                      "NDM",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "network_devices.snmp_traps.forwarder.",
			hostnameEndpointPrefix:        "snmp-traps-intake.",
			intakeTrackType:               "ndmtraps",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
		{
			eventType:                     eventplatform.EventTypeNetworkDevicesNetFlow,
			category:                      "NDM",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "network_devices.netflow.forwarder.",
			hostnameEndpointPrefix:        "ndmflow-intake.",
			intakeTrackType:               "ndmflow",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,

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
			eventType:                     eventplatform.EventTypeNetworkPath,
			category:                      "Network Path",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "network_path.forwarder.",
			hostnameEndpointPrefix:        "netpath-intake.",
			intakeTrackType:               "netpath",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
		{
			eventType:                     eventplatform.EventTypeNetworkConfigManagement,
			category:                      "Network Config Management",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "network_config_management.forwarder.",
			hostnameEndpointPrefix:        "ndm-intake.",
			intakeTrackType:               "ndmconfig",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
		{
			eventType:                     eventplatform.EventTypeContainerLifecycle,
			category:                      "Container",
			contentType:                   logshttp.ProtobufContentType,
			endpointsConfigPrefix:         "container_lifecycle.",
			hostnameEndpointPrefix:        "contlcycle-intake.",
			intakeTrackType:               "contlcycle",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
		{
			eventType:                     eventplatform.EventTypeContainerImages,
			category:                      "Container",
			contentType:                   logshttp.ProtobufContentType,
			endpointsConfigPrefix:         "container_image.",
			hostnameEndpointPrefix:        "contimage-intake.",
			intakeTrackType:               "contimage",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
		{
			eventType:                     eventplatform.EventTypeContainerSBOM,
			category:                      "SBOM",
			contentType:                   logshttp.ProtobufContentType,
			endpointsConfigPrefix:         "sbom.",
			hostnameEndpointPrefix:        "sbom-intake.",
			intakeTrackType:               "sbom",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// on every periodic refresh, we re-send all the SBOMs for all the
			// container images in the workloadmeta store. This can be a lot of
			// payloads at once, so we need a large input channel size to avoid dropping
			defaultInputChanSize: 1000,
		},
		{
			eventType:                     eventplatform.EventTypeSynthetics,
			category:                      "Synthetics",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "synthetics.forwarder.",
			hostnameEndpointPrefix:        "http-synthetics.",
			intakeTrackType:               "synthetics",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
		{
			eventType:                     eventplatform.EventTypeEventManagement,
			category:                      "Event Management",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "event_management.forwarder.",
			hostnameEndpointPrefix:        "event-management-intake.",
			intakeTrackType:               "events",
			defaultBatchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
			//nolint:misspell
			// TODO(ECT-4272): event-management-intake does not support batching/array, must send one event at a time
			useStreamStrategy: true,
		},
		{
			eventType:                     eventTypeDataStreamsMessage,
			category:                      "Data Streams",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "data_streams.forwarder.",
			hostnameEndpointPrefix:        "trace.agent.",
			intakeTrackType:               "data_streams_messages",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		},
	}

	if pkgconfigsetup.Datadog().GetBool("software_inventory.enabled") {
		softinvPipeline := passthroughPipelineDesc{
			eventType:                     eventplatform.EventTypeSoftwareInventory,
			category:                      "EUDM",
			contentType:                   logshttp.JSONContentType,
			endpointsConfigPrefix:         "software_inventory.forwarder.",
			hostnameEndpointPrefix:        "softinv-intake.",
			intakeTrackType:               "softinv",
			defaultBatchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
			defaultBatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			defaultInputChanSize:          pkgconfigsetup.DefaultInputChanSize,
		}
		passthroughPipelineDescs = append(passthroughPipelineDescs, softinvPipeline)
	}

	return passthroughPipelineDescs
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

	// Stream to console if debug mode is enabled
	p.eventPlatformReceiver.HandleMessage(e, []byte{}, eventType)

	select {
	case p.in <- e:
		return nil
	default:
		return fmt.Errorf("event platform forwarder pipeline channel is full for eventType=%s. Channel capacity is %d. consider increasing batch_max_concurrent_send", eventType, cap(p.in))
	}
}

// Diagnose enumerates known epforwarder pipelines and endpoints to test each of them connectivity
func Diagnose() []diagnose.Diagnosis {
	var diagnoses []diagnose.Diagnosis
	cfg := pkgconfigsetup.Datadog()

	for _, desc := range getPassthroughPipelines() {
		//nolint:misspell
		// TODO(ECT-4273): event-management-intake does not support the empty payload sent here
		if desc.eventType == eventplatform.EventTypeEventManagement {
			log.Debugf("Skipping diagnosis for event-management-intake because it does not support the empty payload")
			continue
		}
		configKeys := config.NewLogsConfigKeys(desc.endpointsConfigPrefix, cfg)
		// Use ForDiagnostic variant to avoid registering config update callbacks
		// since these endpoints are transient and will be discarded after the diagnostic check
		endpoints, err := config.BuildEndpointsForDiagnostic(cfg, configKeys, desc.hostnameEndpointPrefix, config.DiagnosticHTTP, desc.intakeTrackType, config.DefaultIntakeProtocol, config.DefaultIntakeOrigin)
		if err != nil {
			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Status:      diagnose.DiagnosisFail,
				Name:        "Endpoints configuration",
				Diagnosis:   "Misconfiguration of agent endpoints",
				Remediation: "Please validate Agent configuration",
				RawError:    err.Error(),
			})
			continue
		}

		url, err := logshttp.CheckConnectivityDiagnose(endpoints.Main, cfg)
		name := "Connectivity to " + url
		if err == nil {
			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Status:    diagnose.DiagnosisSuccess,
				Category:  desc.category,
				Name:      name,
				Diagnosis: fmt.Sprintf("Connectivity to `%s` is Ok", url),
			})
		} else {
			diag := diagnose.Diagnosis{
				Status:      diagnose.DiagnosisFail,
				Category:    desc.category,
				Name:        name,
				Diagnosis:   fmt.Sprintf("Connection to `%s` failed", url),
				Remediation: "Please validate Agent configuration and firewall to access " + url,
				RawError:    err.Error(),
			}
			if cfg.GetBool("convert_dd_site_fqdn.enabled") {
				diag = testConnectivityWithPQDN(cfg, endpoints.Main, diag)
			}
			diagnoses = append(diagnoses, diag)
		}
	}

	return diagnoses
}

// Detect if the connection failed because of using a FQDN by trying with a PQDN
func testConnectivityWithPQDN(cfg model.Reader, endpoint config.Endpoint, diag diagnose.Diagnosis) diagnose.Diagnosis {
	fqdn := endpoint.Host
	if strings.HasSuffix(fqdn, ".") {
		log.Infof("The connection to %s with a FQDN failed; attempting with a PQDN", fqdn)

		// This function takes `endpoint` by value, so it's safe to mutate here
		endpoint.Host = strings.TrimSuffix(fqdn, ".")
		_, err := logshttp.CheckConnectivityDiagnose(endpoint, cfg)
		if err == nil {
			diag.Remediation = fmt.Sprintf(
				"The connection to the fully qualified domain name (FQDN) %q failed, but the connection to %q (without trailing dot) succeeded. Update your firewall and/or proxy configuration to accept FQDN connections, or disable FQDN usage by setting `convert_dd_site_fqdn.enabled` to false in the agent configuration.",
				fqdn, endpoint.Host)
		}
	}
	return diag
}

// SendEventPlatformEventBlocking sends messages to the event platform intake.
// SendEventPlatformEventBlocking will block if the input channel is already full.
func (s *defaultEventPlatformForwarder) SendEventPlatformEventBlocking(e *message.Message, eventType string) error {
	p, ok := s.pipelines[eventType]
	if !ok {
		return fmt.Errorf("unknown eventType=%s", eventType)
	}

	// Stream to console if debug mode is enabled
	p.eventPlatformReceiver.HandleMessage(e, []byte{}, eventType)

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
	sender                *sender.Sender
	strategy              sender.Strategy
	in                    chan *message.Message
	eventPlatformReceiver eventplatformreceiver.Component
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
	forceCompressionKind          string
	forceCompressionLevel         int
	useStreamStrategy             bool
}

// newHTTPPassthroughPipeline creates a new HTTP-only event platform pipeline that sends messages directly to intake
// without any of the processing that exists in regular logs pipelines.
func newHTTPPassthroughPipeline(
	coreConfig model.Reader,
	eventPlatformReceiver eventplatformreceiver.Component,
	compressor logscompression.Component,
	desc passthroughPipelineDesc,
	destinationsContext *client.DestinationsContext,
	pipelineID int,
) (p *passthroughPipeline, err error) {
	configKeys := config.NewLogsConfigKeys(desc.endpointsConfigPrefix, coreConfig)
	compressionOptions := config.EndpointCompressionOptions{
		CompressionKind:  desc.forceCompressionKind,
		CompressionLevel: desc.forceCompressionLevel,
	}
	endpoints, err := config.BuildHTTPEndpointsWithCompressionOverride(
		coreConfig,
		configKeys,
		desc.hostnameEndpointPrefix,
		desc.intakeTrackType,
		config.DefaultIntakeProtocol,
		config.DefaultIntakeOrigin,
		compressionOptions,
	)
	if err != nil {
		return nil, err
	}
	if !endpoints.UseHTTP {
		return nil, errors.New("endpoints must be http")
	}
	// epforwarder pipelines apply their own defaults on top of the hardcoded logs defaults
	if endpoints.BatchMaxConcurrentSend <= 0 {
		endpoints.BatchMaxConcurrentSend = desc.defaultBatchMaxConcurrentSend
	}
	if endpoints.BatchMaxContentSize <= pkgconfigsetup.DefaultBatchMaxContentSize {
		endpoints.BatchMaxContentSize = desc.defaultBatchMaxContentSize
	}
	if endpoints.BatchMaxSize <= pkgconfigsetup.DefaultBatchMaxSize {
		endpoints.BatchMaxSize = desc.defaultBatchMaxSize
	}
	if endpoints.InputChanSize <= pkgconfigsetup.DefaultInputChanSize {
		endpoints.InputChanSize = desc.defaultInputChanSize
	}

	pipelineMonitor := metrics.NewNoopPipelineMonitor(strconv.Itoa(pipelineID))

	inputChan := make(chan *message.Message, endpoints.InputChanSize)

	serverlessMeta := sender.NewServerlessMeta(false)
	senderImpl := httpsender.NewHTTPSender(
		coreConfig,
		&sender.NoopSink{},
		10, // Buffer Size
		serverlessMeta,
		endpoints,
		destinationsContext,
		desc.eventType,
		desc.contentType,
		desc.category,
		sender.DefaultQueuesCount,
		sender.DefaultWorkersPerQueue,
		endpoints.BatchMaxConcurrentSend,
		endpoints.BatchMaxConcurrentSend,
	)

	var encoder compressioncommon.Compressor
	encoder = compressor.NewCompressor("none", 0)
	if endpoints.Main.UseCompression {
		encoder = compressor.NewCompressor(endpoints.Main.CompressionKind, endpoints.Main.CompressionLevel)
	}

	var strategy sender.Strategy

	if desc.useStreamStrategy || desc.contentType == logshttp.ProtobufContentType {
		strategy = sender.NewStreamStrategy(inputChan, senderImpl.In(), encoder)
	} else {
		strategy = sender.NewBatchStrategy(
			inputChan,
			senderImpl.In(),
			make(chan struct{}),
			serverlessMeta,
			endpoints.BatchWait,
			endpoints.BatchMaxSize,
			endpoints.BatchMaxContentSize,
			desc.eventType,
			encoder,
			pipelineMonitor,
			"0",
		)
	}

	log.Debugf("Initialized event platform forwarder pipeline. eventType=%s mainHosts=%s additionalHosts=%s batch_max_concurrent_send=%d batch_max_content_size=%d batch_max_size=%d, input_chan_size=%d, compression_kind=%s, compression_level=%d",
		desc.eventType,
		joinHosts(endpoints.GetReliableEndpoints()),
		joinHosts(endpoints.GetUnReliableEndpoints()),
		endpoints.BatchMaxConcurrentSend,
		endpoints.BatchMaxContentSize,
		endpoints.BatchMaxSize,
		endpoints.InputChanSize,
		endpoints.Main.CompressionKind,
		endpoints.Main.CompressionLevel)
	return &passthroughPipeline{
		sender:                senderImpl,
		strategy:              strategy,
		in:                    inputChan,
		eventPlatformReceiver: eventPlatformReceiver,
	}, nil
}

func (p *passthroughPipeline) Start() {
	if p.strategy != nil {
		p.strategy.Start()
		p.sender.Start()
	}
}

func (p *passthroughPipeline) Stop() {
	if p.strategy != nil {
		p.strategy.Stop()
		p.sender.Stop()
	}
}

func joinHosts(endpoints []config.Endpoint) string {
	var additionalHosts []string
	for _, e := range endpoints {
		additionalHosts = append(additionalHosts, e.Host)
	}
	return strings.Join(additionalHosts, ",")
}

func newDefaultEventPlatformForwarder(config model.Reader, eventPlatformReceiver eventplatformreceiver.Component, compression logscompression.Component) *defaultEventPlatformForwarder {
	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()
	pipelines := make(map[string]*passthroughPipeline)
	for i, desc := range getPassthroughPipelines() {
		p, err := newHTTPPassthroughPipeline(config, eventPlatformReceiver, compression, desc, destinationsCtx, i)
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

type dependencies struct {
	fx.In
	Params                Params
	Config                configcomp.Component
	Lc                    fx.Lifecycle
	EventPlatformReceiver eventplatformreceiver.Component
	Hostname              hostnameinterface.Component
	Compression           logscompression.Component
}

// newEventPlatformForwarder creates a new EventPlatformForwarder
func newEventPlatformForwarder(deps dependencies) eventplatform.Component {
	var forwarder *defaultEventPlatformForwarder

	if deps.Params.UseNoopEventPlatformForwarder {
		forwarder = newNoopEventPlatformForwarder(deps.Hostname, deps.Compression)
	} else if deps.Params.UseEventPlatformForwarder {
		forwarder = newDefaultEventPlatformForwarder(deps.Config, deps.EventPlatformReceiver, deps.Compression)
	}
	if forwarder == nil {
		return option.NonePtr[eventplatform.Forwarder]()
	}
	deps.Lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			forwarder.Start()
			return nil
		},
		OnStop: func(context.Context) error {
			forwarder.Stop()
			return nil
		},
	})
	return option.NewPtr[eventplatform.Forwarder](forwarder)
}

// NewNoopEventPlatformForwarder returns the standard event platform forwarder with sending disabled, meaning events
// will build up in each pipeline channel without being forwarded to the intake
func NewNoopEventPlatformForwarder(hostname hostnameinterface.Component, compression logscompression.Component) eventplatform.Forwarder {
	return newNoopEventPlatformForwarder(hostname, compression)
}

func newNoopEventPlatformForwarder(hostname hostnameinterface.Component, compression logscompression.Component) *defaultEventPlatformForwarder {
	f := newDefaultEventPlatformForwarder(pkgconfigsetup.Datadog(), eventplatformreceiverimpl.NewReceiver(hostname).Comp, compression)
	// remove the senders
	for _, p := range f.pipelines {
		p.strategy = nil
	}
	return f
}

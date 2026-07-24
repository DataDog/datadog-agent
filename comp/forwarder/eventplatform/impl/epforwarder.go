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

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsnoopimpl "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformreceiver "github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/def"
	eventplatformreceiverimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	httpsender "github.com/DataDog/datadog-agent/comp/logs-library/sender/http"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	compressioncommon "github.com/DataDog/datadog-agent/pkg/util/compression"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
)

//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -package=$GOPACKAGE -destination=epforwarder_mockgen.go -build_constraint test

// Requires defines the component's dependencies.
type Requires struct {
	Params                eventplatform.Params
	Config                configcomp.Component
	Lc                    compdef.Lifecycle
	EventPlatformReceiver eventplatformreceiver.Component
	Hostname              hostnameinterface.Component
	Compression           logscompression.Component
	Secrets               secrets.Component
}

// Provides defines the component's output.
type Provides struct {
	Comp eventplatform.Component
}

// NewComponent creates a new EventPlatformForwarder component.
func NewComponent(reqs Requires) Provides {
	return Provides{Comp: newEventPlatformForwarder(reqs)}
}

func getPassthroughPipelines() []passthroughPipelineDesc {
	getters := []func() []passthroughPipelineDesc{
		getDBMPipelines,
		getNDMCorePipelines,
		getNDMIntegrationsPipelines,
		getNetworkPathPipelines,
		getContainerPipelines,
		getSBOMPipelines,
		getGenResourcesPipelines,
		getSyntheticsPipelines,
		getEventManagementPipelines,
		getDataStreamsPipelines,
		getDataObservabilityPipelines,
		getSoftwareInventoryPipelines,
		getAgentDiscoveryPipelines,
		getDataSecurityPipelines,
	}
	var descs []passthroughPipelineDesc
	for _, get := range getters {
		descs = append(descs, get()...)
	}
	return descs
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
		if desc.eventType == eventplatform.EventTypeAgentDiscovery && !cfg.GetBool("config_files_discovery.enabled") {
			continue
		}
		// TODO(dsec): could we diagnose the SDS result pipeline?
		if desc.eventType == eventplatform.EventTypeSDSResult && !cfg.GetBool("data_security.enabled") {
			continue
		}
		//nolint:misspell
		// TODO(ECT-4273): event-management-intake does not support the empty payload sent here
		if desc.eventType == eventplatform.EventTypeEventManagement {
			log.Debugf("Skipping diagnosis for event-management-intake because it does not support the empty payload")
			continue
		}
		if desc.eventType == eventTypeDoQueryResults {
			log.Debugf("Skipping diagnosis for data-obs-intake query-actions because it does not support the empty payload")
			continue
		}
		if desc.eventType == eventplatform.EventTypeKubeActions {
			log.Debugf("Skipping diagnosis for kubeactions-intake because it does not support the empty payload")
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
	hostname string,
	secretsComp secrets.Component,
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

	if desc.eventType == eventTypeDataStreamsMessage {
		tags := fmt.Sprintf("host:%s,agent_version:%s", hostname, version.AgentVersion)
		if taskARN := getECSFargateTaskARN(); taskARN != "" {
			tags += ",task_arn:" + taskARN
		}
		extraHeaders := map[string]string{
			"X-Datadog-Additional-Tags": tags,
		}
		for i := range endpoints.Endpoints {
			endpoints.Endpoints[i].ExtraHTTPHeaders = extraHeaders
		}
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
		secretsComp,
		// Noop: passthrough pipelines don't surface on the logs status page, so they skip
		// utilization sampling and own no snapshot registry.
		pipelineMonitor,
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

// getECSFargateTaskARN returns the ECS task ARN when running on Fargate, or empty string otherwise.
func getECSFargateTaskARN() string {
	if !env.IsECSFargate() {
		return ""
	}
	client, err := ecsmeta.V2()
	if err != nil {
		log.Debugf("Failed to initialize ECS metadata V2 client for task ARN: %v", err)
		return ""
	}
	taskMeta, err := client.GetTask(context.Background())
	if err != nil {
		log.Debugf("Failed to get ECS task metadata for task ARN: %v", err)
		return ""
	}
	return taskMeta.TaskARN
}

func joinHosts(endpoints []config.Endpoint) string {
	var additionalHosts []string
	for _, e := range endpoints {
		additionalHosts = append(additionalHosts, e.Host)
	}
	return strings.Join(additionalHosts, ",")
}

func newDefaultEventPlatformForwarder(config model.Reader, eventPlatformReceiver eventplatformreceiver.Component, compression logscompression.Component, hostname string, secretsComp secrets.Component) *defaultEventPlatformForwarder {
	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()
	pipelines := make(map[string]*passthroughPipeline)
	for i, desc := range getPassthroughPipelines() {
		// TODO(dsec): This could be removed if we want to enable the SDS result pipeline by default.
		if desc.eventType == eventplatform.EventTypeSDSResult && !config.GetBool("data_security.enabled") {
			continue
		}
		p, err := newHTTPPassthroughPipeline(config, eventPlatformReceiver, compression, desc, destinationsCtx, i, hostname, secretsComp)
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

func newEventPlatformForwarder(reqs Requires) eventplatform.Component {
	var forwarder *defaultEventPlatformForwarder

	if reqs.Params.UseNoopEventPlatformForwarder {
		forwarder = newNoopEventPlatformForwarder(reqs.Hostname, reqs.Compression)
	} else if reqs.Params.UseEventPlatformForwarder {
		hostnameStr := reqs.Hostname.GetSafe(context.Background())
		forwarder = newDefaultEventPlatformForwarder(reqs.Config, reqs.EventPlatformReceiver, reqs.Compression, hostnameStr, reqs.Secrets)
	}
	if forwarder == nil {
		return option.NonePtr[eventplatform.Forwarder]()
	}
	reqs.Lc.Append(compdef.Hook{
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
	hostnameStr := hostname.GetSafe(context.Background())
	f := newDefaultEventPlatformForwarder(pkgconfigsetup.Datadog(), eventplatformreceiverimpl.NewReceiver(hostname, pkgconfigsetup.Datadog()).Comp, compression, hostnameStr, secretsnoopimpl.NewComponent().Comp)
	// remove the senders
	for _, p := range f.pipelines {
		p.strategy = nil
	}
	return f
}

package epforwarder

import (
	"fmt"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"strings"
	"sync"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	eventTypeDBMSamples = "dbm-samples"
	eventTypeDBMMetrics = "dbm-metrics"

	// EventTypeNetworkDevicesMetadata is the event type for network devices metadata
	EventTypeNetworkDevicesMetadata = "network-devices-metadata"
)

var passthroughPipelineDescs = []passthroughPipelineDesc{
	{
		eventType:              eventTypeDBMSamples,
		endpointsConfigPrefix:  "database_monitoring.samples.",
		hostnameEndpointPrefix: "dbquery-http-intake.logs.",
		// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
		defaultBatchMaxConcurrentSend: 10,
		defaultBatchMaxContentSize:    pkgconfig.DefaultBatchMaxContentSize,
		defaultBatchMaxSize:           pkgconfig.DefaultBatchMaxSize,
	},
	{
		eventType:              eventTypeDBMMetrics,
		endpointsConfigPrefix:  "database_monitoring.metrics.",
		hostnameEndpointPrefix: "dbm-metrics-intake.",
		// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
		defaultBatchMaxConcurrentSend: 10,
		defaultBatchMaxContentSize:    20e6,
		defaultBatchMaxSize:           pkgconfig.DefaultBatchMaxSize,
	},
	{
		eventType:                     EventTypeNetworkDevicesMetadata,
		endpointsConfigPrefix:         "network_devices.metadata.",
		hostnameEndpointPrefix:        "network-devices.",
		defaultBatchMaxConcurrentSend: 10,
		defaultBatchMaxContentSize:    pkgconfig.DefaultBatchMaxContentSize,
		defaultBatchMaxSize:           pkgconfig.DefaultBatchMaxSize,
	},
}

// An EventPlatformForwarder forwards Messages to a destination based on their event type
type EventPlatformForwarder interface {
	SendEventPlatformEvent(e *message.Message, eventType string) error
	Purge() map[string][]*message.Message
	Start()
	Stop()
}

type defaultEventPlatformForwarder struct {
	purgeMx         sync.Mutex
	pipelines       map[string]*passthroughPipeline
	destinationsCtx *client.DestinationsContext
}

func (s *defaultEventPlatformForwarder) SendEventPlatformEvent(e *message.Message, eventType string) error {
	p, ok := s.pipelines[eventType]
	if !ok {
		return fmt.Errorf("unknown eventType=%s", eventType)
	}
	select {
	case p.in <- e:
		return nil
	default:
		return fmt.Errorf("event platform forwarder pipeline channel is full for eventType=%s. consider increasing batch_max_concurrent_send", eventType)
	}
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
		result[eventType] = purgeChan(p.in)
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
	stopper := restart.NewParallelStopper()
	for _, p := range s.pipelines {
		stopper.Add(p)
	}
	stopper.Stop()
	// TODO: wait on stop and cancel context only after timeout like logs agent
	s.destinationsCtx.Stop()
	log.Debugf("event platform forwarder shut down complete")
}

type passthroughPipeline struct {
	sender  *sender.Sender
	in      chan *message.Message
	auditor auditor.Auditor
}

type passthroughPipelineDesc struct {
	eventType                     string
	endpointsConfigPrefix         string
	hostnameEndpointPrefix        string
	defaultBatchMaxConcurrentSend int
	defaultBatchMaxContentSize    int
	defaultBatchMaxSize           int
}

// newHTTPPassthroughPipeline creates a new HTTP-only event platform pipeline that sends messages directly to intake
// without any of the processing that exists in regular logs pipelines.
func newHTTPPassthroughPipeline(desc passthroughPipelineDesc, destinationsContext *client.DestinationsContext) (p *passthroughPipeline, err error) {
	configKeys := config.NewLogsConfigKeys(desc.endpointsConfigPrefix, coreConfig.Datadog)
	endpoints, err := config.BuildHTTPEndpointsWithConfig(configKeys, desc.hostnameEndpointPrefix)
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
	main := http.NewDestination(endpoints.Main, http.JSONContentType, destinationsContext, endpoints.BatchMaxConcurrentSend)
	additionals := []client.Destination{}
	for _, endpoint := range endpoints.Additionals {
		additionals = append(additionals, http.NewDestination(endpoint, http.JSONContentType, destinationsContext, endpoints.BatchMaxConcurrentSend))
	}
	destinations := client.NewDestinations(main, additionals)
	inputChan := make(chan *message.Message, 100)
	strategy := sender.NewBatchStrategy(sender.ArraySerializer, endpoints.BatchWait, endpoints.BatchMaxConcurrentSend, pkgconfig.DefaultBatchMaxSize, endpoints.BatchMaxContentSize, desc.eventType)
	a := auditor.NewNullAuditor()
	log.Debugf("Initialized event platform forwarder pipeline. eventType=%s mainHost=%s additionalHosts=%s batch_max_concurrent_send=%d batch_max_content_size=%d batch_max_size=%d",
		desc.eventType, endpoints.Main.Host, joinHosts(endpoints.Additionals), endpoints.BatchMaxConcurrentSend, endpoints.BatchMaxContentSize, endpoints.BatchMaxSize)
	return &passthroughPipeline{
		sender:  sender.NewSender(inputChan, a.Channel(), destinations, strategy),
		in:      inputChan,
		auditor: a,
	}, nil
}

func (p *passthroughPipeline) Start() {
	p.auditor.Start()
	if p.sender != nil {
		p.sender.Start()
	}
}

func (p *passthroughPipeline) Stop() {
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
	for _, desc := range passthroughPipelineDescs {
		p, err := newHTTPPassthroughPipeline(desc, destinationsCtx)
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
		p.sender = nil
	}
	return f
}

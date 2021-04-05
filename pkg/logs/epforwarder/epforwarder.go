package epforwarder

import (
	"fmt"
	"strings"

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
)

// An EventPlatformForwarder forwards Messages to a destination based on their event type
type EventPlatformForwarder interface {
	SendEventPlatformEvent(e *message.Message, eventType string) error
	Start()
	Stop()
}

type defaultEventPlatformForwarder struct {
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

// newHTTPPassthroughPipeline creates a new HTTP-only event platform pipeline that sends messages directly to intake
// without any of the processing that exists in regular logs pipelines.
func newHTTPPassthroughPipeline(
	eventType string,
	endpointsConfigPrefix string,
	hostnameEndpointPrefix string,
	destinationsContext *client.DestinationsContext) (p *passthroughPipeline, err error) {
	defer func() {
		if err != nil {
			log.Errorf("Failed to initialize event platform forwarder pipeline. eventType=%s, error=%s", eventType, err.Error())
		}
	}()
	configKeys := config.LogsConfigKeys{
		CompressionLevel:        endpointsConfigPrefix + ".compression_level",
		ConnectionResetInterval: endpointsConfigPrefix + ".connection_reset_interval",
		LogsDDURL:               endpointsConfigPrefix + ".logs_dd_url",
		DDURL:                   endpointsConfigPrefix + ".dd_url",
		DevModeNoSSL:            endpointsConfigPrefix + ".dev_mode_no_ssl",
		AdditionalEndpoints:     endpointsConfigPrefix + ".additional_endpoints",
		BatchWait:               endpointsConfigPrefix + ".batch_wait",
		BatchMaxConcurrentSend:  endpointsConfigPrefix + ".batch_max_concurrent_send",
	}
	endpoints, err := config.BuildHTTPEndpointsWithConfig(configKeys, hostnameEndpointPrefix)
	if err != nil {
		return nil, err
	}
	if !endpoints.UseHTTP {
		return nil, fmt.Errorf("endpoints must be http")
	}
	// since some of these pipelines can be potentially very high throughput we increase the default batch send concurrency
	// to ensure they are able to support 1000s of events per second
	if endpoints.BatchMaxConcurrentSend <= 0 {
		endpoints.BatchMaxConcurrentSend = 10
	}
	main := http.NewDestination(endpoints.Main, http.JSONContentType, destinationsContext, endpoints.BatchMaxConcurrentSend)
	additionals := []client.Destination{}
	for _, endpoint := range endpoints.Additionals {
		additionals = append(additionals, http.NewDestination(endpoint, http.JSONContentType, destinationsContext, endpoints.BatchMaxConcurrentSend))
	}
	destinations := client.NewDestinations(main, additionals)
	inputChan := make(chan *message.Message, 100)
	strategy := sender.NewBatchStrategy(sender.ArraySerializer, endpoints.BatchWait, endpoints.BatchMaxConcurrentSend)
	a := auditor.NewNullAuditor()
	log.Debugf("Initialized event platform forwarder pipeline. eventType=%s mainHost=%s additionalHosts=%s batch_max_concurrent_send=%d", eventType, endpoints.Main.Host, joinHosts(endpoints.Additionals), endpoints.BatchMaxConcurrentSend)
	return &passthroughPipeline{
		sender:  sender.NewSender(inputChan, a.Channel(), destinations, strategy),
		in:      inputChan,
		auditor: a,
	}, nil
}

func (p *passthroughPipeline) Start() {
	p.auditor.Start()
	p.sender.Start()
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

// NewEventPlatformForwarder creates a new EventPlatformForwarder
func NewEventPlatformForwarder() EventPlatformForwarder {
	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()
	pipelines := make(map[string]*passthroughPipeline)

	if p, err := newHTTPPassthroughPipeline(eventTypeDBMSamples, "database_monitoring.samples", "dbquery-http-intake.logs.", destinationsCtx); err == nil {
		pipelines[eventTypeDBMSamples] = p
	}
	if p, err := newHTTPPassthroughPipeline(eventTypeDBMMetrics, "database_monitoring.metrics", "dbmetrics-http-intake.logs.", destinationsCtx); err == nil {
		pipelines[eventTypeDBMMetrics] = p
	}

	return &defaultEventPlatformForwarder{
		pipelines:       pipelines,
		destinationsCtx: destinationsCtx,
	}
}

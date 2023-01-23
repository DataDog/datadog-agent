package telemetry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/trace/config"

	"go.uber.org/atomic"
)

const (
	GenericError = 1
)

// OnboardingEvent is an isntrumentation telemetry onboarding event type of payload
type OnboardingEvent struct {
	RequestType string                 `json:"request_type"`
	ApiVersion  string                 `json:"api_version"`
	Payload     OnboardingEventPayload `json:"payload,omitempty"`
}

// OnboardingEventPayload ...
type OnboardingEventPayload struct {
	EventName string               `json:"event_name"`
	Tags      OnboardingEventTags  `json:"tags"`
	Error     OnboardingEventError `json:"error,omitempty"`
}

// OnboardingEventTags ...
type OnboardingEventTags struct {
	AgentPlatform string `json:"agent_platform,omitempty"`
	AgentVersion  string `json:"agent_version,omitempty"`
	AgentHostname string `json:"agent_hostname,omitempty"`
	Env           string `json:"env,omitempty"`
}

// OnboardingEventError ...
type OnboardingEventError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type TelemetryCollector interface {
	Start()
	Stop()
	SendStartupSuccess()
	SendStartupError(code int, err error, sync bool)
}

// TelemetryForwarder ...
type telemetryCollector struct {
	in     chan *OnboardingEvent
	done   chan struct{}
	client *config.ResetClient

	endpoints             []config.Endpoint
	userAgent             string
	cfg                   *config.AgentConfig
	collectedStartupError *atomic.Bool
}

// NewCollector returns either forwarder, or a noop implementation if instrumentation telemetry is disabled
func NewCollector(cfg *config.AgentConfig) TelemetryCollector {
	if cfg.TelemetryConfig.Enabled {
		return &noopTelemetryCollector{}
	}

	var endpoints []config.Endpoint
	for _, endpoint := range cfg.TelemetryConfig.Endpoints {
		u, err := url.Parse(endpoint.Host)
		if err != nil {
			continue
		}
		u.Path = "/v2/apmtelemetry"
		endpointWithPath := *endpoint
		endpointWithPath.Host = u.String()

		endpoints = append(endpoints, endpointWithPath)
	}

	return &telemetryCollector{
		in:        make(chan *OnboardingEvent, 1000),
		done:      make(chan struct{}),
		client:    cfg.NewHTTPClient(),
		endpoints: endpoints,
		userAgent: fmt.Sprintf("Datadog Trace Agent/%s/%s", cfg.AgentVersion, cfg.GitCommit),

		cfg:                   cfg,
		collectedStartupError: &atomic.Bool{},
	}
}

// NewNoopCollector returns a noop collector
func NewNoopCollector() TelemetryCollector {
	return &noopTelemetryCollector{}
}

// Start ...
func (f *telemetryCollector) Start() {
	go f.loop()
}

func (f *telemetryCollector) loop() {
	for event := range f.in {
		f.sendEvent(event)
	}
	close(f.done)
}

func (f *telemetryCollector) sendEvent(event *OnboardingEvent) {
	for _, endpoint := range f.endpoints {
		body := bytes.NewBuffer(nil)
		if err := json.NewEncoder(body).Encode(event); err != nil {
			continue
		}

		req, err := http.NewRequest("POST", endpoint.Host, body)
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("User-Agent", f.userAgent)
		req.Header.Add("DD-Api-Key", endpoint.APIKey)
		req.Header.Add("Content-Length", strconv.Itoa(body.Len()))

		if err != nil {
			continue
		}
		resp, err := f.client.Do(req)
		if err != nil {
			continue
		}
		// Inconditionally read the body and ignore any errors
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

// Stop ...
func (f *telemetryCollector) Stop() {
	close(f.in)
	<-f.done
}

func NewOnboardingTelemetryPayload(config *config.AgentConfig) OnboardingEvent {
	return OnboardingEvent{
		RequestType: "apm-onboarding-event",
		ApiVersion:  "v1",
		Payload: OnboardingEventPayload{
			Tags: OnboardingEventTags{
				AgentVersion:  config.AgentVersion,
				AgentHostname: config.Hostname,
				Env:           config.DefaultEnv,
			},
		},
	}
}

func (f *telemetryCollector) SendStartupSuccess() {
	if f.collectedStartupError.Load() {
		return
	}
	ev := NewOnboardingTelemetryPayload(f.cfg)
	ev.Payload.EventName = "agent.startup.success"
	f.queueEvent(&ev)
}

func (f *telemetryCollector) SendStartupError(code int, err error, sync bool) {
	f.collectedStartupError.Store(true)
	ev := NewOnboardingTelemetryPayload(f.cfg)
	ev.Payload.EventName = "agent.startup.error"
	ev.Payload.Error.Code = code
	ev.Payload.Error.Message = err.Error()
	if sync {
		f.sendEvent(&ev)
	} else {
		f.queueEvent(&ev)
	}
}

func (f *telemetryCollector) queueEvent(e *OnboardingEvent) {
	f.in <- e
}

type noopTelemetryCollector struct{}

func (*noopTelemetryCollector) Start()                                          {}
func (*noopTelemetryCollector) Stop()                                           {}
func (*noopTelemetryCollector) SendStartupSuccess()                             {}
func (*noopTelemetryCollector) SendStartupError(code int, err error, sync bool) {}

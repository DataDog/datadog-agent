package telemetry

import (
	"encoding/json"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
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
	SendStartupError(code int, err error)
}

// TelemetryForwarder ...
type telemetryCollector struct {
	forwarder             epforwarder.EventPlatformForwarder
	config                *config.AgentConfig
	collectedStartupError atomic.Bool
}

// NewCollector returns either forwarder, or a noop implementation if instrumentation telemetry is disabled
func NewCollector(config *config.AgentConfig) TelemetryCollector {
	if config.TelemetryConfig.Enabled {
		return &noopTelemetryCollector{}
	}
	return &telemetryCollector{
		forwarder: epforwarder.NewTraceAgentEventPlatformForwarder(),
		config:    config,
	}
}

// NewNoopCollector returns a noop collector
func NewNoopCollector() TelemetryCollector {
	return &noopTelemetryCollector{}
}

// Start ...
func (f *telemetryCollector) Start() {
	f.forwarder.Start()
}

// Stop ...
func (f *telemetryCollector) Stop() {
	f.forwarder.Stop()
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
	ev := NewOnboardingTelemetryPayload(f.config)
	ev.Payload.EventName = "agent.startup.success"
	f.sendOnboardingEvent(&ev)
}

func (f *telemetryCollector) SendStartupError(code int, err error) {
	f.collectedStartupError.Store(true)
	ev := NewOnboardingTelemetryPayload(f.config)
	ev.Payload.EventName = "agent.startup.error"
	ev.Payload.Error.Code = code
	ev.Payload.Error.Message = err.Error()
	f.sendOnboardingEvent(&ev)
}

func (f *telemetryCollector) sendOnboardingEvent(e *OnboardingEvent) {
	content, err := json.Marshal(e)
	if err != nil {
		return
	}
	m := message.NewMessage(content, nil, message.StatusInfo, 0)
	err = f.forwarder.SendEventPlatformEvent(m, epforwarder.EventTypeInstrumentatiomTelemetry)
	if err != nil {
		return
	}
}

type noopTelemetryCollector struct{}

func (*noopTelemetryCollector) Start()                               {}
func (*noopTelemetryCollector) Stop()                                {}
func (*noopTelemetryCollector) SendStartupSuccess()                  {}
func (*noopTelemetryCollector) SendStartupError(code int, err error) {}

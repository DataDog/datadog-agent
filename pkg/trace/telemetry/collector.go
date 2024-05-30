// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(TEL) Fix revive linter
package telemetry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"

	"go.uber.org/atomic"
)

// Error codes associated with each startup error
// The full list, and associated description is contained in the Tracking APM Onboarding RFC
const (
	GenericError         = 1
	CantCreateLogger     = 8
	TraceAgentNotEnabled = 9
	CantWritePIDFile     = 10
	// CantSetupAutoExit is no longer used. To avoid conflicting issues in the future
	// rather than removing from the codebase we will keep it here so no one use 11 for a new error code.
	// CantSetupAutoExit      = 11
	CantConfigureDogstatsd = 12
	CantCreateRCCLient     = 13
	//nolint:revive // TODO(TEL) Fix revive linter
	CantStartHttpServer        = 14
	CantStartUdsServer         = 15
	CantStartWindowsPipeServer = 16
	InvalidIntakeEndpoint      = 17
)

// The agent will try to send a "first trace sent" event up to 5 times.
// If all five retries fail, it will not send any more of them to avoid
// spamming the Datadog backend.
const maxFirstTraceFailures = 5

// OnboardingEvent contains
type OnboardingEvent struct {
	RequestType string `json:"request_type"`
	//nolint:revive // TODO(TEL) Fix revive linter
	ApiVersion string                 `json:"api_version"`
	Payload    OnboardingEventPayload `json:"payload,omitempty"`
}

// OnboardingEventPayload ...
type OnboardingEventPayload struct {
	EventName string               `json:"event_name"`
	Tags      OnboardingEventTags  `json:"tags"`
	Error     OnboardingEventError `json:"error,omitempty"`
}

// OnboardingEventTags ...
type OnboardingEventTags struct {
	// v1+
	InstallID     string `json:"install_id,omitempty"`
	InstallType   string `json:"install_type,omitempty"`
	InstallTime   int64  `json:"install_time,omitempty"`
	AgentPlatform string `json:"agent_platform,omitempty"`
	AgentVersion  string `json:"agent_version,omitempty"`
	AgentHostname string `json:"agent_hostname,omitempty"`
	Env           string `json:"env,omitempty"`

	// v2+
	Architecture  string `json:"architecture,omitempty"`
	ServiceName   string `json:"service_name,omitempty"`
	LibLanguage   string `json:"lib_language,omitempty"`
	LibVersion    string `json:"lib_version,omitempty"`
	InjectionTime int64  `json:"injection_time,omitempty"`
}

var errReceivedUnsuccessfulStatusCode = fmt.Errorf("received a 4XX or 5xx error code while submitting telemetry data")

// OnboardingEventError ...
type OnboardingEventError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// TelemetryCollector is the interface used to send reports about startup to the instrumentation telemetry intake
//
//nolint:revive // TODO(TEL) Fix revive linter
type TelemetryCollector interface {
	SendStartupSuccess()
	SendStartupError(code int, err error)
	SentFirstTrace() bool
	SendFirstTrace()
	SendLibInjectionAttempted(metadata LibInjectionMetadata)
}

type telemetryCollector struct {
	client *config.ResetClient

	endpoints             []config.Endpoint
	userAgent             string
	cfg                   *config.AgentConfig
	collectedStartupError *atomic.Bool
	collectedFirstTrace   *atomic.Bool
	firstTraceFailures    *atomic.Int32
}

// NewCollector returns either collector, or a noop implementation if instrumentation telemetry is disabled
func NewCollector(cfg *config.AgentConfig) TelemetryCollector {
	if !cfg.TelemetryConfig.Enabled {
		return &noopTelemetryCollector{}
	}

	var endpoints []config.Endpoint
	for _, endpoint := range cfg.TelemetryConfig.Endpoints {
		u, err := url.Parse(endpoint.Host)
		if err != nil {
			continue
		}
		u.Path = "/api/v2/apmtelemetry"
		endpointWithPath := *endpoint
		endpointWithPath.Host = u.String()

		endpoints = append(endpoints, endpointWithPath)
	}

	return &telemetryCollector{
		client:    cfg.NewHTTPClient(),
		endpoints: endpoints,
		userAgent: fmt.Sprintf("Datadog Trace Agent/%s/%s", cfg.AgentVersion, cfg.GitCommit),

		cfg:                   cfg,
		collectedStartupError: &atomic.Bool{},
		collectedFirstTrace:   &atomic.Bool{},
		firstTraceFailures:    &atomic.Int32{},
	}
}

func (f *telemetryCollector) sendEvent(event *OnboardingEvent) (err error) {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	bodyLen := strconv.Itoa(len(body))
	for _, endpoint := range f.endpoints {
		req, reqErr := http.NewRequest("POST", endpoint.Host, bytes.NewReader(body))
		if reqErr != nil {
			err = reqErr
			continue
		}
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("User-Agent", f.userAgent)
		req.Header.Add("DD-Api-Key", endpoint.APIKey)
		req.Header.Add("Content-Length", bodyLen)

		resp, reqErr := f.client.Do(req)
		if reqErr != nil {
			err = reqErr
			continue
		}
		// Unconditionally read the body and ignore any errors
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 400 {
			err = errReceivedUnsuccessfulStatusCode
		}
	}
	return err
}

type onboardingEventOption func(e *OnboardingEvent)

func withVersion(version string) onboardingEventOption {
	return func(e *OnboardingEvent) {
		e.ApiVersion = version
	}
}

func withEventName(name string) onboardingEventOption {
	return func(e *OnboardingEvent) {
		e.Payload.EventName = name
	}
}

func withError(code int, err error) onboardingEventOption {
	return func(e *OnboardingEvent) {
		e.Payload.Error.Code = code
		e.Payload.Error.Message = err.Error()
	}
}

func newOnboardingTelemetryPayload(config *config.AgentConfig, options ...onboardingEventOption) OnboardingEvent {
	ev := OnboardingEvent{
		RequestType: "apm-onboarding-event",
		ApiVersion:  "v1", // default
		Payload: OnboardingEventPayload{
			Tags: OnboardingEventTags{
				AgentVersion:  config.AgentVersion,
				AgentHostname: config.Hostname,
				Env:           config.DefaultEnv,
			},
		},
	}

	if config.InstallSignature.Found {
		ev.Payload.Tags.InstallID = config.InstallSignature.InstallID
		ev.Payload.Tags.InstallType = config.InstallSignature.InstallType
		ev.Payload.Tags.InstallTime = config.InstallSignature.InstallTime
	}

	for _, option := range options {
		option(&ev)
	}

	return ev
}

func (f *telemetryCollector) SendStartupSuccess() {
	if f.collectedStartupError.Load() {
		return
	}
	//nolint:errcheck // TODO(TEL) Fix errcheck linter
	f.sendEventPayload(
		withEventName("agent.startup.success"),
	)
}

func (f *telemetryCollector) SentFirstTrace() bool {
	swapped := f.collectedFirstTrace.CompareAndSwap(false, true)
	return !swapped
}

func (f *telemetryCollector) SendFirstTrace() {
	if err := f.sendEventPayload(
		withEventName("agent.first_trace.sent"),
	); err != nil {
		if f.firstTraceFailures.Inc() < maxFirstTraceFailures {
			f.collectedFirstTrace.Store(false)
		}
	}
}

type LibInjectionMetadata struct {
	ServiceName   string
	LibLanguage   string
	LibVersion    string
	InjectionTime time.Time
}

func withLibInjectionMetadata(m LibInjectionMetadata) onboardingEventOption {
	return func(e *OnboardingEvent) {
		e.Payload.Tags.ServiceName = m.ServiceName
		e.Payload.Tags.LibLanguage = m.LibLanguage
		e.Payload.Tags.LibVersion = m.LibVersion
		e.Payload.Tags.InjectionTime = m.InjectionTime.Unix()
	}
}

func (f *telemetryCollector) SendLibInjectionAttempted(params LibInjectionMetadata) {
	//nolint:errcheck // TODO(TEL) Fix errcheck linter
	f.sendEventPayload(
		withVersion("v2"),
		withEventName("service.library_injection.attempted"),
		withLibInjectionMetadata(params),
	)
}

func (f *telemetryCollector) SendStartupError(code int, err error) {
	f.collectedStartupError.Store(true)
	//nolint:errcheck // TODO(TEL) Fix errcheck linter
	f.sendEventPayload(
		withEventName("agent.startup.error"),
		withError(code, err),
	)
}

func (f *telemetryCollector) eventPayload(option ...onboardingEventOption) OnboardingEvent {
	return newOnboardingTelemetryPayload(f.cfg, option...)
}

func (f *telemetryCollector) sendEventPayload(option ...onboardingEventOption) error {
	ev := f.eventPayload(option...)
	return f.sendEvent(&ev)
}

// NewNoopCollector returns a noop collector
func NewNoopCollector() TelemetryCollector {
	return &noopTelemetryCollector{}
}

type noopTelemetryCollector struct{}

//nolint:revive // TODO(TEL) Fix revive linter
func (*noopTelemetryCollector) SendStartupSuccess()                                     {}
func (*noopTelemetryCollector) SendStartupError(code int, err error)                    {}
func (*noopTelemetryCollector) SendFirstTrace()                                         {}
func (*noopTelemetryCollector) SentFirstTrace() bool                                    { return true }
func (*noopTelemetryCollector) SendLibInjectionAttempted(metadata LibInjectionMetadata) {}

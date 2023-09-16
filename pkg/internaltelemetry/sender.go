// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package internaltelemetry full description in README.md
package internaltelemetry

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
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"go.uber.org/atomic"
)

type LogTelemetrySender interface {
	SendLog(level, message string)
}

type logTelemetrySender struct {
	client *config.ResetClient

	endpoints             []config.Endpoint
	userAgent             string
	cfg                   *config.AgentConfig
	collectedStartupError *atomic.Bool
}

// LogEvent exported so it can be turned into json
type LogEvent struct {
	ApiVersion  string         `json:"api_version"`
	RequestType string         `json:"request_type"` // should always be logs
	TracerTime  int64          `json:"tracer_time"`  // unix timestamp (in seconds)
	RuntimeId   string         `json:"runtime_id"`
	SequenceId  int            `json:"seq_id"`
	DebugFlag   bool           `json:"debug"`
	Host        HostPayload    `json:"host"`
	Payload     LogPayload     `json:"payload"`
	Application LogApplication `json:"application"`
	//	Host LogHost `json:"host"`
}

type HostPayload struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Arch          string `json:"architecture"`
	KernelName    string `json:"kernel_name"`
	KernelRelease string `json:"kernel_release"`
	KernelVersion string `json:"kernel_version"`
}
type LogMessage struct {
	Message string `json:"message"`
	Level   string `json:"level"`
}
type LogPayload struct {
	Logs []LogMessage `json:"logs"`
}
type LogApplication struct {
	ServiceName     string `json:"service_name"`
	LanguageName    string `json:"language_name"`
	LanguageVersion string `json:"language_version"`
	TracerVersion   string `json:"tracer_version"`
}
type LogHost struct {
}

var (
	msgSequenceId = int(1) // will increment on every send
)

// NewLogTelemetrySender returns either collector, or a noop implementation if instrumentation telemetry is disabled
func NewLogTelemetrySender(cfg *config.AgentConfig) LogTelemetrySender {

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

	return &logTelemetrySender{
		client:    cfg.NewHTTPClient(),
		endpoints: endpoints,
		userAgent: fmt.Sprintf("Datadog Trace Agent/%s/%s", cfg.AgentVersion, cfg.GitCommit),

		cfg:                   cfg,
		collectedStartupError: &atomic.Bool{},
	}
}

func formatMessage(level, message string) *LogEvent {
	sq := msgSequenceId // todo, this is racy
	msgSequenceId++

	le := &LogEvent{
		ApiVersion:  "v2",
		RequestType: "logs",
		TracerTime:  time.Now().Unix(),
		DebugFlag:   true,
		RuntimeId:   "a",
		SequenceId:  sq,
	}
	h := HostPayload{
		Hostname:      "WD-2016DBG", //todo
		OS:            "windows",
		Arch:          "x64",
		KernelName:    "windows",
		KernelRelease: "2016",
		KernelVersion: "10.0.0.0",
	}
	lm := LogMessage{
		Message: message,
		Level:   level,
	}
	app := LogApplication{
		ServiceName:     "ddnpm",
		LanguageName:    "go", //"agent",
		LanguageVersion: "1.20",
		TracerVersion:   "7.49",
	}
	lp := LogPayload{}
	lp.Logs = append(lp.Logs, lm)
	le.Payload = lp
	le.Application = app
	le.Host = h
	return le
}
func (lts *logTelemetrySender) SendLog(level, message string) {

	le := formatMessage(level, message)
	body, err := json.Marshal(le)
	if err != nil {
		return
	}
	bodylen := strconv.Itoa(len(body))
	log.Infof("%v", string(body))
	for _, endpoint := range lts.endpoints {
		req, err := http.NewRequest("POST", endpoint.Host, bytes.NewReader(body))
		if err != nil {
			continue
		}
		lts.addTelemetryHeaders(req, endpoint.APIKey, bodylen)
		resp, err := lts.client.Do(req)
		if err != nil {
			continue
		}
		// Unconditionally read the body and ignore any errors
		log.Infof("%v %v", resp.Status, resp.StatusCode)
		_, _ = io.Copy(io.Discard, resp.Body)

		resp.Body.Close()
	}

}

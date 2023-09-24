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

	metadatautils "github.com/DataDog/datadog-agent/comp/metadata/host/utils"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"

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

	// we can pre-calculate the host payload structure at init time
	logEvent LogEvent
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
func NewLogTelemetrySender(cfg *config.AgentConfig, svcname, langname string) LogTelemetrySender {

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

	av, _ := version.Agent()
	info := metadatautils.GetInformation()

	return &logTelemetrySender{
		client:    cfg.NewHTTPClient(),
		endpoints: endpoints,
		userAgent: fmt.Sprintf("Datadog Trace Agent/%s/%s", cfg.AgentVersion, cfg.GitCommit),

		cfg:                   cfg,
		collectedStartupError: &atomic.Bool{},
		logEvent: LogEvent{
			ApiVersion:  "v2",
			RequestType: "logs",
			DebugFlag:   true,
			RuntimeId:   info.HostID,
			Host: HostPayload{
				Hostname:      info.Hostname,
				OS:            info.OS,
				Arch:          info.KernelArch,
				KernelName:    info.Platform,
				KernelRelease: info.PlatformVersion,
				KernelVersion: info.PlatformVersion,
			},
			Application: LogApplication{
				ServiceName:     svcname,
				LanguageName:    langname,
				LanguageVersion: av.GetNumberAndPre(),
				TracerVersion:   av.GetNumberAndPre(),
			},
		},
	}
}

func (lts *logTelemetrySender) formatMessage(level, message string) *LogEvent {
	sq := msgSequenceId // todo, this is racy
	msgSequenceId++

	le := lts.logEvent // take all the prepoulated values.
	le.TracerTime = time.Now().Unix()
	le.SequenceId = sq

	lm := LogMessage{
		Message: message,
		Level:   level,
	}
	lp := LogPayload{}
	lp.Logs = append(lp.Logs, lm)
	le.Payload = lp
	return &le
}
func (lts *logTelemetrySender) SendLog(level, message string) {

	le := lts.formatMessage(level, message)
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/shirou/gopsutil/v4/host"
)

const (
	telemetryEndpoint         = "/api/v2/apmtelemetry"
	defaultSendPayloadTimeout = time.Second * 10
)

type endpoint struct {
	APIKey string `json:"-"`
	Host   string
}

type client struct {
	m                  sync.Mutex
	client             httpClient
	endpoints          []*endpoint
	sendPayloadTimeout time.Duration

	// we can pre-calculate the host payload structure at init time
	baseEvent event
}

type requestType string

const (
	requestTypeLogs   requestType = "logs"
	requestTypeTraces requestType = "traces"
)

type event struct {
	APIVersion  string      `json:"api_version"`
	RequestType requestType `json:"request_type"`
	TracerTime  int64       `json:"tracer_time"` // unix timestamp (in seconds)
	RuntimeID   string      `json:"runtime_id"`
	SequenceID  uint64      `json:"seq_id"`
	DebugFlag   bool        `json:"debug"`
	Origin      string      `json:"origin"`
	Host        hostInfo    `json:"host"`
	Application application `json:"application"`
	Payload     interface{} `json:"payload"`
}

type hostInfo struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Arch          string `json:"architecture"`
	KernelName    string `json:"kernel_name"`
	KernelRelease string `json:"kernel_release"`
	KernelVersion string `json:"kernel_version"`
}

type application struct {
	ServiceName     string `json:"service_name"`
	ServiceVersion  string `json:"service_version"`
	LanguageName    string `json:"language_name"`
	LanguageVersion string `json:"language_version"`
	TracerVersion   string `json:"tracer_version"`
}

type tracePayload struct {
	Traces []trace `json:"traces"`
}

type traces []trace

type trace []*span

// span used for installation telemetry
type span struct {
	// Service is the name of the service that handled this span.
	Service string `json:"service"`
	// Name is the name of the operation this span represents.
	Name string `json:"name"`
	// Resource is the name of the resource this span represents.
	Resource string `json:"resource"`
	// TraceID is the ID of the trace to which this span belongs.
	TraceID uint64 `json:"trace_id"`
	// SpanID is the ID of this span.
	SpanID uint64 `json:"span_id"`
	// ParentID is the ID of the parent span.
	ParentID uint64 `json:"parent_id"`
	// Start is the start time of this span in nanoseconds since the Unix epoch.
	Start int64 `json:"start"`
	// Duration is the duration of this span in nanoseconds.
	Duration int64 `json:"duration"`
	// Error is the error status of this span.
	Error int32 `json:"error"`
	// Meta is a mapping from tag name to tag value for string-valued tags.
	Meta map[string]string `json:"meta,omitempty"`
	// Metrics is a mapping from metric name to metric value for numeric metrics.
	Metrics map[string]float64 `json:"metrics,omitempty"`
	// Type is the type of the span.
	Type string `json:"type"`
}

// LogPayload defines the log payload object
type LogPayload struct {
	Logs []LogMessage `json:"logs"`
}

// LogMessage defines the log message object
type LogMessage struct {
	Message string `json:"message"`
	Level   string `json:"level"`
}

var sequenceID atomic.Uint64

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func newClient(httpClient httpClient, endpoints []*endpoint, service string, debug bool) *client {
	info, err := host.Info()
	if err != nil {
		log.Warnf("failed to retrieve host info: %v", err)
		info = &host.InfoStat{}
	}
	return &client{
		client:             httpClient,
		endpoints:          endpoints,
		sendPayloadTimeout: defaultSendPayloadTimeout,
		baseEvent: event{
			APIVersion: "v2",
			DebugFlag:  debug,
			RuntimeID:  info.HostID,
			Origin:     "agent",
			Host: hostInfo{
				Hostname:      info.Hostname,
				OS:            info.OS,
				Arch:          info.KernelArch,
				KernelName:    info.Platform,
				KernelRelease: info.PlatformVersion,
				KernelVersion: info.PlatformVersion,
			},
			Application: application{
				ServiceName:     service,
				ServiceVersion:  version.AgentVersion,
				LanguageName:    "go",
				LanguageVersion: runtime.Version(),
				TracerVersion:   "n/a",
			},
		},
	}
}

func (c *client) SendLog(level, message string) {
	c.m.Lock()
	defer c.m.Unlock()
	payload := LogPayload{
		Logs: []LogMessage{{Message: message, Level: level}},
	}
	c.sendPayload(requestTypeLogs, payload)
}

func (c *client) SendTraces(traces traces) {
	c.m.Lock()
	defer c.m.Unlock()
	payload := tracePayload{
		Traces: c.sampleTraces(traces),
	}
	c.sendPayload(requestTypeTraces, payload)
}

// sampleTraces is a simple uniform sampling function that samples traces based
// on the sampling rate, given that there is no trace agent to sample the traces
// We try to keep the tracer behaviour: the first rule that matches apply its rate to the whole trace
func (c *client) sampleTraces(ts traces) traces {
	tracesWithSampling := traces{}
	for _, trace := range ts {
		samplingRate := 1.0
		for _, span := range trace {
			if val, ok := span.Metrics["_dd.rule_psr"]; ok {
				samplingRate = val
				break
			}
		}
		if rand.Float64() < samplingRate {
			tracesWithSampling = append(tracesWithSampling, trace)
		}
	}
	log.Debugf("sampling telemetry traces (had %d, kept %d)", len(ts), len(tracesWithSampling))
	return tracesWithSampling
}

func (c *client) sendPayload(requestType requestType, payload interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), c.sendPayloadTimeout)
	defer cancel()

	event := c.baseEvent
	event.RequestType = requestType
	event.SequenceID = sequenceID.Add(1)
	event.TracerTime = time.Now().Unix()
	event.Payload = payload

	serializedPayload, err := json.Marshal(event)
	if err != nil {
		log.Errorf("failed to serialize payload: %v", err)
		return
	}
	group := sync.WaitGroup{}
	for _, e := range c.endpoints {
		group.Add(1)
		go func(e *endpoint) {
			defer group.Done()
			url := fmt.Sprintf("%s%s", e.Host, telemetryEndpoint)
			req, err := http.NewRequest("POST", url, bytes.NewReader(serializedPayload))
			if err != nil {
				log.Errorf("failed to create request for endpoint %s: %v", url, err)
				return
			}
			req.Header.Add("dd-api-key", e.APIKey)
			req.Header.Add("content-type", "application/json")
			req.Header.Add("dd-telemetry-api-version", "v2")
			req.Header.Add("dd-telemetry-request-type", string(event.RequestType))
			req.Header.Add("dd-telemetry-origin", event.Origin)
			req.Header.Add("dd-client-library-language", event.Application.LanguageName)
			req.Header.Add("dd-client-library-version", event.Application.TracerVersion)
			req.Header.Add("dd-telemetry-debug-enabled", strconv.FormatBool(event.DebugFlag))
			req.Header.Add("dd-agent-hostname", event.Host.Hostname)

			resp, err := c.client.Do(req.WithContext(ctx))
			if err != nil {
				log.Warnf("failed to send telemetry payload to endpoint %s: %v", url, err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				log.Warnf("failed to send telemetry payload to endpoint %s: %s", url, resp.Status)
			}
			_, _ = io.Copy(io.Discard, resp.Body)

		}(e)
	}
	group.Wait()
}

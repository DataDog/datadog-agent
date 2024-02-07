// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package agenttelemetryimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
	dto "github.com/prometheus/client_model/go"
)

const (
	telemetryEndpointPrefix = "https://instrumentation-telemetry-intake."

	httpClientResetInterval = 5 * time.Minute
	httpClientTimeout       = 10 * time.Second
	maximumNumberOfPayloads = 50
)

// ---------------
// interfaces
type sender interface {
	startSession(cancelCtx context.Context) *senderSession
	flushSession(ss *senderSession) error
	sendAgentStatusPayload(session *senderSession, agentStatusPayload map[string]interface{}) error
	sendAgentMetricPayloads(session *senderSession, metrics []*agentmetric) error
}

type client interface {
	Do(req *http.Request) (*http.Response, error)
}

// ---------------
// senderImpl
type senderImpl struct {
	cfgComp  config.Reader
	logComp  log.Component
	hostComp host.Component

	client client

	endpointURL  string
	agentVersion string

	// pre-fill parts of payload which are not changing during run-time
	payloadTemplate             Payload
	metadataPayloadTemplate     AgentMetadataPayload
	agentMetricsPayloadTemplate AgentMetricsPayload
	agentStatusPayloadTemplate  AgentStatusPayload
}

// HostPayload defines the host payload object. It is currently used only as payload's header
// and it is not stored with backend. It could be removed in the future completly. It is expected
// by backend to be present in the payload and currently tootaly reducted.
type HostPayload struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Arch          string `json:"architecture"`
	KernelName    string `json:"kernel_name"`
	KernelRelease string `json:"kernel_release"`
	KernelVersion string `json:"kernel_version"`
}

// AgentMetadataPayload should be top level object in the payload but currently tucked into specific payloads
// until backend will be adjusted properly
type AgentMetadataPayload struct {
	HostID string `json:"hostid"`
}

// Payload defines the top level object in the payload
type Payload struct {
	APIVersion  string      `json:"api_version"`
	RequestType string      `json:"request_type"`
	EventTime   int64       `json:"event_time"`
	DebugFlag   bool        `json:"debug"`
	Host        HostPayload `json:"host"`
	Payload     interface{} `json:"payload"`
}

// ---------------
// BATCH PAYLOAD WRAPPER
//
// part of payload batching
type payloadInfo struct {
	requestType string
	payload     interface{}
}

// senderSession is also use to batch payloads
type senderSession struct {
	cancelCtx context.Context
	payloads  []payloadInfo
}

// BatchPayloadWrapper exported so it can be turned into json
type BatchPayloadWrapper struct {
	RequestType string      `json:"request_type"`
	Payload     interface{} `json:"payload"`
}

// ---------------
// AGENT METRICS
//

// AgentMetricsPayload defines Metrics object
type AgentMetricsPayload struct {
	Message string                 `json:"message"`
	Metrics map[string]interface{} `json:"metrics"`
}

// MetricPayload defines Metric object
type MetricPayload struct {
	Value   float64                `json:"value"`
	Type    string                 `json:"type"`
	Tags    map[string]interface{} `json:"tags,omitempty"`
	Buckets map[string]interface{} `json:"buckets,omitempty"`
}

// -------------
// AGENT STATUS
//

// AgentStatusPayload defines AgentStatus object
type AgentStatusPayload struct {
	Message     string                 `json:"message"`
	AgentStatus map[string]interface{} `json:"agent_status"`
}

func httpClientFactory(cfg config.Reader, timeout time.Duration) func() *http.Client {
	return func() *http.Client {
		return &http.Client{
			Timeout: timeout,
			// reusing core agent HTTP transport to benefit from proxy settings.
			Transport: httputils.CreateHTTPTransport(cfg),
		}
	}
}

func newSenderClientImpl(agentCfg config.Component) client {
	return httputils.NewResetClient(httpClientResetInterval, httpClientFactory(agentCfg, httpClientTimeout))
}

func newSenderImpl(
	cfgComp config.Component,
	logComp log.Component,
	hostComp host.Component,
	client client) (sender, error) {
	// Form Agent Telemetry endpoint
	endpoint := utils.GetMainEndpoint(cfgComp, telemetryEndpointPrefix, "dd_url")
	endpointURL, err := url.JoinPath(endpoint, "/api/v2/apmtelemetry")
	if err != nil {
		return nil, fmt.Errorf("failed to form agent telemetry endpoint URL from configuration: %v", err)
	}

	// Get host information (only hostid is used for now)
	info := hostComp.GetInformation()

	// Redact all host info for now until we have a proper way to handle it
	host := HostPayload{
		Hostname:      "[REDACTED]", // info.Hostname,
		OS:            "[REDACTED]", // info.OS,
		Arch:          "[REDACTED]", // info.KernelArch,
		KernelName:    "[REDACTED]", // info.Platform,
		KernelRelease: "[REDACTED]", // info.PlatformVersion,
		KernelVersion: "[REDACTED]", // info.KernelVersion,
	}

	agentVersion, _ := version.Agent()

	return &senderImpl{
		cfgComp:  cfgComp,
		logComp:  logComp,
		hostComp: hostComp,

		client:       client,
		endpointURL:  endpointURL,
		agentVersion: agentVersion.GetNumberAndPre(),
		// pre-fill parts of payload which are not changing during run-time
		payloadTemplate: Payload{
			APIVersion: "v2",
			DebugFlag:  false,
			Host:       host,
		},
		metadataPayloadTemplate: AgentMetadataPayload{
			HostID: info.HostID,
		},
		agentMetricsPayloadTemplate: AgentMetricsPayload{
			Message: "Agent metrics",
		},
		agentStatusPayloadTemplate: AgentStatusPayload{
			Message: "Agent status",
		},
	}, nil
}

func (s *senderImpl) addMetricPayload(
	metricName string,
	metricFamily *dto.MetricFamily,
	metric *dto.Metric,
	metricsPayload *AgentMetricsPayload) {

	// Copy template
	payload := MetricPayload{}

	// Add metric value
	metricType := metricFamily.GetType()
	switch metricType {
	case dto.MetricType_COUNTER:
		payload.Type = "monotonic"
		payload.Value = metric.GetCounter().GetValue()
	case dto.MetricType_GAUGE:
		payload.Type = "gauge"
		payload.Value = metric.GetGauge().GetValue()
	case dto.MetricType_HISTOGRAM:
		payload.Type = "histogram"
		payload.Buckets = make(map[string]interface{}, 0)
		histogram := metric.GetHistogram()
		for _, bucket := range histogram.GetBucket() {
			boundName := fmt.Sprintf("upperbound_%v", bucket.GetUpperBound())
			payload.Buckets[boundName] = bucket.GetCumulativeCount()
		}
	}

	// Add metric tags
	if len(metric.GetLabel()) != 0 {
		payload.Tags = make(map[string]interface{}, 0)
		for _, labelPair := range metric.GetLabel() {
			payload.Tags[labelPair.GetName()] = labelPair.GetValue()
		}
	}

	// Finally add metric to the payload
	metricsPayload.Metrics[metricName] = payload
}

func (s *senderImpl) startSession(cancelCtx context.Context) *senderSession {
	return &senderSession{
		cancelCtx: cancelCtx,
	}
}

func (s *senderImpl) flushSession(ss *senderSession) error {
	// There is nothing to do if there are no payloads
	if len(ss.payloads) == 0 {
		return nil
	}

	s.logComp.Infof("Flushing Agent Telemetery session with %d payloads", len(ss.payloads))

	// Defer cleanup of payloads. Even if there is an error, we want to cleanup
	// but in future we may want to add retry logic.
	defer func() {
		ss.payloads = nil
	}()

	// Create a payload with a single message or batch of messages
	payload := s.payloadTemplate
	payload.EventTime = time.Now().Unix()
	if len(ss.payloads) == 1 {
		// Single payload will be sent directly using the request type of the payload
		payload.RequestType = ss.payloads[0].requestType
		payload.Payload = ss.payloads[0].payload
	} else {
		// Batch up multiple payloads into single "batch" payload type
		payload.RequestType = "message-batch"
		payloadWrappers := make([]BatchPayloadWrapper, 0)
		for _, p := range ss.payloads {
			payloadWrappers = append(payloadWrappers,
				BatchPayloadWrapper{
					RequestType: p.requestType,
					Payload:     p.payload,
				})
		}
		payload.Payload = payloadWrappers
	}

	// Marshal the payload to a byte array
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Send the payload
	req, err := http.NewRequest("POST", s.endpointURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	s.addHeaders(req, payload.RequestType, s.cfgComp.GetString("api_key"), strconv.Itoa(len(reqBody)))
	resp, err := s.client.Do(req.WithContext(ss.cancelCtx))
	if err != nil {
		return err
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()

	// Log return status
	s.logComp.Infof("Telemetery request response status: %s, status code: %d", resp.Status, resp.StatusCode)

	return nil
}

func (s *senderImpl) sendAgentMetricPayloads(session *senderSession, metrics []*agentmetric) error {
	// Are there any metrics
	if len(metrics) == 0 {
		return nil
	}

	// Create one or more metric payloads batching different metrics into a single payload,
	// but the same metric (with multiple tag sets) into different payloads. This is needed
	// to avoid creating JSON payloads which contains arrays (otherwise we could not
	// effectively query and or aggregate these values). Metrics are batchup where first
	// slice of all tag sets goes to first payload entry, second to second, etc. Effectively
	// we are creating a "transposed" matrix of metrics and tag sets, where each
	// message/payload contains multiples metrics for a single index of tag set. Essentially
	// the number of message/payloads is equal to the maximum number of tag sets for a single
	// metric.
	var payloads []*AgentMetricsPayload
	for _, am := range metrics {
		for idx, m := range am.filteredMetrics {
			var payload *AgentMetricsPayload

			// reuse or add a payload
			if idx+1 > len(payloads) {
				newPayload := s.agentMetricsPayloadTemplate
				newPayload.Metrics = make(map[string]interface{}, 0)
				newPayload.Metrics["agent_metadata"] = s.metadataPayloadTemplate
				payloads = append(payloads, &newPayload)
			}
			payload = payloads[idx]
			s.addMetricPayload(am.metricName, am.promMetricFamily, m, payload)
		}
	}

	// We will batch multiples metrics payloads into single "batch" payload type
	// but for now send it one by one
	for _, payload := range payloads {
		if err := s.sendPayload(session, "agent-metrics", payload); err != nil {
			return err
		}
	}

	return nil
}

func (s *senderImpl) sendAgentStatusPayload(session *senderSession, agentStatusPayload map[string]interface{}) error {
	payload := s.agentStatusPayloadTemplate
	payload.AgentStatus = agentStatusPayload
	payload.AgentStatus["agent_metadata"] = s.metadataPayloadTemplate

	return s.sendPayload(session, "agent-status", payload)
}

func (s *senderImpl) sendPayload(session *senderSession, requestType string, payload interface{}) error {
	// Add payload to session
	session.payloads = append(session.payloads, payloadInfo{requestType, payload})

	// Flush session if it is full
	if len(session.payloads) >= maximumNumberOfPayloads {
		return s.flushSession(session)
	}

	return nil
}

func (s *senderImpl) addHeaders(req *http.Request, requesttype, apikey, bodylen string) {
	req.Header.Add("DD-Api-Key", apikey)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Content-Length", bodylen)
	req.Header.Add("DD-Telemetry-api-version", "v2")
	req.Header.Add("DD-Telemetry-request-type", requesttype)
	req.Header.Add("DD-Telemetry-Product", "agent")
	req.Header.Add("DD-Telemetry-Product-Version", s.agentVersion)
	// Not clear how to acquire that. Appears that EVP adds it automatically
	req.Header.Add("datadog-container-id", "")
}

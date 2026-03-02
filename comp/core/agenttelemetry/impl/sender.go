// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package agenttelemetryimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	hostinfoutils "github.com/DataDog/datadog-agent/pkg/util/hostinfo"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	telemetryEndpointPrefix         = "https://instrumentation-telemetry-intake."
	telemetryConfigPrefix           = "agent_telemetry."
	telemetryHostnameEndpointPrefix = "instrumentation-telemetry-intake."
	telemetryIntakeTrackType        = "agenttelemetry"
	telemetryPath                   = "/api/v2/apmtelemetry"

	metricPayloadType = "agent-metrics"
	batchPayloadType  = "message-batch"

	httpClientResetInterval = 5 * time.Minute
	httpClientTimeout       = 10 * time.Second
)

// ---------------
// interfaces
type sender interface {
	startSession(cancelCtx context.Context) *senderSession
	flushSession(ss *senderSession) error

	sendAgentMetricPayloads(ss *senderSession, metrics []*agentmetric)
	sendEventPayload(ss *senderSession, eventInfo *Event, eventPayload map[string]interface{})
}

type client interface {
	Do(req *http.Request) (*http.Response, error)
}

// ---------------
// senderImpl
type senderImpl struct {
	cfgComp config.Reader
	logComp log.Component

	compress         bool
	compressionLevel int
	client           client

	endpoints *logconfig.Endpoints

	agentVersion string

	// pre-fill parts of payload which are not changing during run-time
	payloadTemplate             Payload
	metadataPayloadTemplate     AgentMetadataPayload
	agentMetricsPayloadTemplate AgentMetricsPayload
}

// HostPayload defines the host payload object. It is currently used only as payload's header
// and it is not stored with backend. It could be removed in the future completly. It is expected
// by backend to be present in the payload and currently tootaly reducted.
type HostPayload struct {
	Hostname string `json:"hostname"`
}

// AgentMetadataPayload should be top level object in the payload but currently tucked into specific payloads
// until backend will be adjusted properly
type AgentMetadataPayload struct {
	HostID   string `json:"hostid"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	OSVer    string `json:"osver"`
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

// senderSession store and seriaizes one or more payloads
type senderSession struct {
	cancelCtx       context.Context
	payloadTemplate Payload

	// metric payloads
	metricPayloads []*AgentMetricsPayload

	// event payload
	eventInfo    *Event
	eventPayload map[string]interface{}
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
	Buckets map[string]uint64      `json:"buckets,omitempty"`
	P75     *float64               `json:"p75,omitempty"`
	P95     *float64               `json:"p95,omitempty"`
	P99     *float64               `json:"p99,omitempty"`
}

// -------------------
// Utilities
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

// buils url from a config endpoint.
func buildURL(endpoint logconfig.Endpoint) string {
	var address string
	if endpoint.Port != 0 {
		address = fmt.Sprintf("%v:%v", endpoint.Host, endpoint.Port)
	} else {
		address = endpoint.Host
	}

	url := url.URL{
		Scheme: "https",
		Host:   address,
		Path:   endpoint.PathPrefix + telemetryPath,
	}

	return url.String()
}

func getEndpoints(cfgComp config.Component) (*logconfig.Endpoints, error) {
	// borrowed and styled after EP Forwarder newHTTPPassthroughPipeline().
	// Will be eliminated in the future after switching to EP Forwarder.
	configKeys := logconfig.NewLogsConfigKeys(telemetryConfigPrefix, cfgComp)
	return logconfig.BuildHTTPEndpointsWithConfig(cfgComp, configKeys,
		telemetryHostnameEndpointPrefix, telemetryIntakeTrackType, logconfig.DefaultIntakeProtocol, logconfig.DefaultIntakeOrigin)
}

func newSenderImpl(
	cfgComp config.Component,
	logComp log.Component,
	client client) (sender, error) {

	// "Sending" part of the sender will be moved to EP Forwarder in the future to be able
	// to support retry, caching, URL management, API key rotation at runtime, flush to
	// disk, backoff logic, etc. There are few nuances needs to be adopted by EP Forwarder
	// to support Agent Telemetry:
	//   * Support custom HTTP headers
	//   * Support indication of main Endpoint selection/filtering if there are more than one
	//   * Potentially/optionally support custom batching of payloads (custom batching envelope)
	//
	//  When ported to EP Forwarder we will need to send each telemetry type on a separate pipeline.
	endpoints, err := getEndpoints(cfgComp)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent telemetry endpoints: %v", err)
	}

	// Get host information (only hostid is used for now)
	info := hostinfoutils.GetInformation()

	// Complying with intake schema by providing dummy data (may change in the future)
	host := HostPayload{
		Hostname: "x",
	}

	agentVersion, _ := version.Agent()

	return &senderImpl{
		cfgComp: cfgComp,
		logComp: logComp,

		compress:         cfgComp.GetBool("agent_telemetry.use_compression"),
		compressionLevel: cfgComp.GetInt("agent_telemetry.compression_level"),
		client:           client,
		endpoints:        endpoints,
		agentVersion:     agentVersion.GetNumberAndPre(),

		// pre-fill parts of payload which are not changing during run-time
		payloadTemplate: Payload{
			APIVersion: "v2",
			DebugFlag:  false,
			Host:       host,
		},
		metadataPayloadTemplate: AgentMetadataPayload{
			HostID:   info.HostID,
			Hostname: info.Hostname,
			OS:       info.OS,
			OSVer:    info.PlatformVersion,
		},
		agentMetricsPayloadTemplate: AgentMetricsPayload{
			Message: "Agent metrics",
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
		payload.Type = "counter"
		payload.Value = metric.GetCounter().GetValue()
	case dto.MetricType_GAUGE:
		payload.Type = "gauge"
		payload.Value = metric.GetGauge().GetValue()
	case dto.MetricType_HISTOGRAM:
		payload.Type = "histogram"
		payload.Buckets = make(map[string]uint64, 0)
		histogram := metric.GetHistogram()
		for _, bucket := range histogram.GetBucket() {
			boundNameRaw := fmt.Sprintf("%v", bucket.GetUpperBound())
			boundName := strings.ReplaceAll(boundNameRaw, ".", "_")

			payload.Buckets[boundName] = bucket.GetCumulativeCount()
		}
		payload.Buckets["+Inf"] = histogram.GetSampleCount()

		// Calculate fixed 75, 95 and 99 precentiles. Percentile calculation finds
		// a bucket which, with all preceding buckets, contains that percentile item.
		// For convenience, percentile values are not the bucket number but its
		// upper-bound. If a percentile belongs to the implicit "+inf" bucket, which
		// has no explicit upper-bound, we will use the last bucket upper bound times 2.
		// The upper-bound of the "+Inf" bucket is defined as 2x of the preceding
		// bucket boundary, but it is totally arbitrary. In the future we may use a
		// configuration value to set it up.
		var totalCount uint64
		for _, bucket := range histogram.GetBucket() {
			totalCount += bucket.GetCumulativeCount()
		}
		totalCount += histogram.GetSampleCount()
		p75 := uint64(math.Floor(float64(totalCount) * 0.75))
		p95 := uint64(math.Floor(float64(totalCount) * 0.95))
		p99 := uint64(math.Floor(float64(totalCount) * 0.99))
		var curCount uint64
		for _, bucket := range histogram.GetBucket() {
			curCount += bucket.GetCumulativeCount()
			if payload.P75 == nil && curCount >= p75 {
				p75Value := bucket.GetUpperBound()
				payload.P75 = &p75Value
			}
			if payload.P95 == nil && curCount >= p95 {
				p95Value := bucket.GetUpperBound()
				payload.P95 = &p95Value
			}
			if payload.P99 == nil && curCount >= p99 {
				p99Value := bucket.GetUpperBound()
				payload.P99 = &p99Value
			}
		}
		maxUpperBound := 2 * (histogram.GetBucket()[len(histogram.GetBucket())-1].GetUpperBound())
		if payload.P75 == nil {
			payload.P75 = &maxUpperBound
		}
		if payload.P95 == nil {
			payload.P95 = &maxUpperBound
		}
		if payload.P99 == nil {
			payload.P99 = &maxUpperBound
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
		cancelCtx:       cancelCtx,
		payloadTemplate: s.payloadTemplate,
	}
}

func (ss *senderSession) payloadCount() int {
	payloadCount := len(ss.metricPayloads)
	if ss.eventPayload != nil {
		payloadCount++
	}
	return payloadCount
}

func (ss *senderSession) flush() Payload {
	defer func() {
		// Clear payloads when done
		ss.metricPayloads = nil
		ss.eventInfo = nil
		ss.eventPayload = nil
	}()

	// Create a payload with a single message or batch of messages
	payload := ss.payloadTemplate
	payload.EventTime = time.Now().Unix()

	// Create top-level event payload if needed
	var eventWrapPayload map[string]interface{}
	if ss.eventPayload != nil {
		eventWrapPayload = make(map[string]interface{})
		eventWrapPayload["message"] = ss.eventInfo.Message
		eventWrapPayload[ss.eventInfo.PayloadKey] = ss.eventPayload
	}

	if ss.payloadCount() == 1 {
		// Either metric or event payload (single payload will be sent directly using the request type of the payload)
		if len(ss.metricPayloads) == 1 {
			mp := ss.metricPayloads[0]
			payload.RequestType = metricPayloadType
			payload.Payload = mp
		} else {
			payload.RequestType = ss.eventInfo.RequestType
			payload.Payload = eventWrapPayload
		}
	} else {
		// Batch up multiple payloads into single "batch" payload type
		batch := make([]BatchPayloadWrapper, 0)
		for _, mp := range ss.metricPayloads {
			batch = append(batch,
				BatchPayloadWrapper{
					RequestType: metricPayloadType,
					Payload:     payloadInfo{metricPayloadType, mp}.payload,
				})
		}
		// add event payload if present
		if ss.eventPayload != nil {
			batch = append(batch,
				BatchPayloadWrapper{
					RequestType: ss.eventInfo.RequestType,
					Payload:     eventWrapPayload,
				})
		}
		payload.RequestType = batchPayloadType
		payload.Payload = batch
	}

	return payload
}

func (s *senderImpl) flushSession(ss *senderSession) error {
	// There is nothing to do if there are no payloads
	if ss.payloadCount() == 0 {
		return nil
	}

	s.logComp.Debugf("Flushing Agent Telemetery session with %d payloads", ss.payloadCount())

	payloads := ss.flush()
	payloadJSON, err := json.Marshal(payloads)
	if err != nil {
		return fmt.Errorf("failed to marshal agent telemetry payload: %w", err)
	}

	reqBodyRaw, err := scrubber.ScrubJSON(payloadJSON)
	if err != nil {
		return fmt.Errorf("failed to scrubl agent telemetry payload: %w", err)
	}

	// Try to compress the payload if needed
	reqBody := reqBodyRaw
	compressed := false
	if s.compress {
		// In case of failed to compress continue with uncompress body
		reqBodyCompressed, errTemp := zstdCompressLevel(reqBodyRaw, s.compressionLevel)
		if errTemp == nil {
			compressed = true
			reqBody = reqBodyCompressed
		} else {
			s.logComp.Warnf("Failed to compress agent telemetry payload: %v", errTemp)
		}
	}

	// Send the payload to all endpoints
	var errs error
	reqType := payloads.RequestType
	bodyLen := strconv.Itoa(len(reqBody))
	for _, ep := range s.endpoints.Endpoints {
		url := buildURL(ep)
		req, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		s.addHeaders(req, reqType, ep.GetAPIKey(), bodyLen, compressed)
		resp, err := s.client.Do(req.WithContext(ss.cancelCtx))
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		defer func() {
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
		}()

		// Log return status (and URL if unsuccessful)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			s.logComp.Debugf("Telemetry endpoint response status:%s, request type:%s, status code:%d", resp.Status, reqType, resp.StatusCode)
		} else {
			s.logComp.Debugf("Telemetry endpoint response status:%s, request type:%s, status code:%d, url:%s", resp.Status, reqType, resp.StatusCode, url)
		}
	}

	return errs
}

func (s *senderImpl) sendAgentMetricPayloads(ss *senderSession, metrics []*agentmetric) {
	// Create one or more metric payloads batching different metrics into a single payload,
	// but the same metric (with multiple tag sets) into different payloads. This is needed
	// to avoid creating JSON payloads which contains arrays (otherwise we could not
	// effectively query and or aggregate these values). Metrics are batchup where first
	// slice of all tag sets goes to first payload entry, second to second, etc. Effectively
	// we are creating a "transposed" matrix of metrics and tag sets, where each
	// message/payload contains multiples metrics for a single index of tag set. Essentially
	// the number of message/payloads is equal to the maximum number of tag sets for a single
	// metric.
	for _, am := range metrics {
		for idx, m := range am.metrics {
			var payload *AgentMetricsPayload

			// reuse or add a payload
			if idx+1 > len(ss.metricPayloads) {
				newPayload := s.agentMetricsPayloadTemplate
				newPayload.Metrics = make(map[string]interface{}, 0)
				newPayload.Metrics["agent_metadata"] = s.metadataPayloadTemplate
				ss.metricPayloads = append(ss.metricPayloads, &newPayload)
			}
			payload = ss.metricPayloads[idx]
			s.addMetricPayload(am.name, am.family, m, payload)
		}
	}
}

func (s *senderImpl) sendEventPayload(ss *senderSession, eventInfo *Event, eventPayload map[string]interface{}) {
	ss.eventInfo = eventInfo
	ss.eventPayload = eventPayload
	ss.eventPayload["agent_metadata"] = s.metadataPayloadTemplate
}

func (s *senderImpl) addHeaders(req *http.Request, requesttype, apikey, bodylen string, compressed bool) {
	req.Header.Add("DD-Api-Key", apikey)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Content-Length", bodylen)
	req.Header.Add("DD-Telemetry-api-version", "v2")
	req.Header.Add("DD-Telemetry-request-type", requesttype)
	req.Header.Add("DD-Telemetry-Product", "agent")
	req.Header.Add("DD-Telemetry-Product-Version", s.agentVersion)
	// Not clear how to acquire that. Appears that EVP adds it automatically
	req.Header.Add("datadog-container-id", "")

	if compressed {
		req.Header.Set("Content-Encoding", "zstd")
	}
}

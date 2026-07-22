// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// telemetryRequestTypeHeader names the header tracer libraries use to declare
// the kind of telemetry payload they are sending.
const telemetryRequestTypeHeader = "DD-Telemetry-Request-Type"

// apmTelemetryRequestType is the value of telemetryRequestTypeHeader used by
// the SSI telemetry forwarder to report a single injection attempt.
const apmTelemetryRequestType = "injection-metadata"

// apmTelemetryProxyPath is the request path (after /telemetry/proxy is
// stripped by the route mux) that carries APM library telemetry.
const apmTelemetryProxyPath = "/api/v2/apmtelemetry"

// sensitiveArgFlags are flag names whose value must be redacted even when
// passed as a separate argv token from the flag (e.g. "--password hunter2")
// rather than joined with "=" or ":", which pkg/util/scrubber's default
// replacers already handle. Mirrors the word list process-agent's cmdline
// scrubber (pkg/process/procutil.DataScrubber) uses; that package can't be
// imported here because pkg/trace is a standalone Go module and procutil
// lives in the (much larger) root module.
var sensitiveArgFlags = []string{
	"password", "passwd", "pwd", "mysql_pwd",
	"access_token", "auth_token", "token",
	"api_key", "apikey", "secret", "credentials",
}

func newCmdLineScrubber() *scrubber.Scrubber {
	s := scrubber.NewWithDefaults()
	for _, word := range sensitiveArgFlags {
		pattern := strings.ReplaceAll(regexp.QuoteMeta(word), "_", "[-_]")
		re := regexp.MustCompile(`(?i)((?:-{1,2})?` + pattern + `)( +)([^\s]+)`)
		s.AddReplacer(scrubber.SingleLine, scrubber.Replacer{
			Regex: re,
			Repl:  []byte(`$1$2********`),
		})
	}
	return s
}

// telemetryRequest is a partial decode of the APM library telemetry envelope
// (see https://github.com/DataDog/instrumentation-telemetry-api-docs). Only
// the field carrying the typed payload needs to be addressable; the rest are
// kept as raw JSON so re-marshalling does not drop or reorder tracer-supplied
// fields the agent does not care about.
type telemetryRequest struct {
	APIVersion  string          `json:"api_version"`
	RequestType string          `json:"request_type"`
	TracerTime  int64           `json:"tracer_time"`
	RuntimeID   string          `json:"runtime_id"`
	SeqID       int64           `json:"seq_id"`
	Application json.RawMessage `json:"application"`
	Host        json.RawMessage `json:"host"`
	Payload     json.RawMessage `json:"payload"`
	Debug       bool            `json:"debug,omitempty"`
}

// injectionMetadata is the payload sent inside a telemetryRequest with
// request_type=injection-metadata by the SSI tracer-injection sidecar.
type injectionMetadata struct {
	Component        string          `json:"component"`
	ComponentVersion string          `json:"component_version"`
	Result           string          `json:"result"`
	ResultReason     string          `json:"result_reason"`
	ResultClass      string          `json:"result_class"`
	RuntimeID        string          `json:"runtime_id"`
	CommandLine      string          `json:"command_line"`
	TimestampMillis  int64           `json:"timestamp_millis"`
	CreateTimeMillis int64           `json:"create_time_millis"`
	Language         string          `json:"language"`
	Metadata         json.RawMessage `json:"metadata,omitempty"`
}

// patchJSONField re-encodes the JSON object raw, replacing only the named
// top-level field with value, leaving every other field — including any
// unknown to this package — byte-for-byte equivalent. This avoids the data
// loss that decoding into a fixed struct and re-marshalling it would cause
// whenever the sender (a newer tracer or SSI sidecar) includes fields this
// package does not model.
func patchJSONField(raw []byte, field string, value json.RawMessage) ([]byte, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	obj[field] = value
	return json.Marshal(obj)
}

const originTag = "origin"

type cloudResourceType string
type cloudProvider string

const (
	awsFargate                    cloudResourceType = "AWSFargate"
	cloudRun                      cloudResourceType = "GCPCloudRun"
	cloudFunctions                cloudResourceType = "GCPCloudFunctions"
	azureAppService               cloudResourceType = "AzureAppService"
	azureContainerApp             cloudResourceType = "AzureContainerApp"
	aws                           cloudProvider     = "AWS"
	gcp                           cloudProvider     = "GCP"
	azure                         cloudProvider     = "Azure"
	cloudProviderHeader           string            = "dd-cloud-provider"
	cloudResourceTypeHeader       string            = "dd-cloud-resource-type"
	cloudResourceIdentifierHeader string            = "dd-cloud-resource-identifier"
)

// This number was chosen because requests on the EVP are accepted with sizes up to 5Mb, so we
// want to be able to buffer at least a few max size requests before exerting backpressure.
//
// And using 25Mb at most per host seems not too unreasonnable.
//
// Looking at payload size distribution, the max requests we get is about than 1Mb anyway,
// the biggest p99 per language is around 350Kb for nodejs and p95 is around 13Kb.
// So it should provide enough in normal cases before we start dropping requests.
const maxInflightBytes = 25 * 1000 * 1000

const maxConcurrentRequests = 20

const maxInflightRequests = 100

// TelemetryForwarder sends HTTP requests to multiple targets.
// The handler returns immediately and the forwarding is done in the background.
//
// To provide somne backpressure, we limit the number of concurrent forwarded requests
type TelemetryForwarder struct {
	endpoints []*config.Endpoint
	conf      *config.AgentConfig

	forwardedReqChan chan forwardedRequest
	inflightWaiter   sync.WaitGroup
	inflightCount    atomic.Int64
	maxInflightBytes int64

	cancelCtx context.Context
	cancelFn  context.CancelFunc
	done      chan struct{}

	containerIDProvider IDProvider
	client              *config.ResetClient
	statsd              statsd.ClientInterface
	logger              *log.ThrottledLogger
	cmdLineScrubber     *scrubber.Scrubber
}

// NewTelemetryForwarder creates a new TelemetryForwarder
func NewTelemetryForwarder(conf *config.AgentConfig, containerIDProvider IDProvider, statsd statsd.ClientInterface) *TelemetryForwarder {
	// extract and validate Hostnames from configured endpoints
	var endpoints []*config.Endpoint
	if conf.TelemetryConfig != nil {
		for _, endpoint := range conf.TelemetryConfig.Endpoints {
			u, err := url.Parse(endpoint.Host)
			if err != nil {
				log.Errorf("Error parsing apm_config.telemetry endpoint %q: %v", endpoint.Host, err)
				continue
			}
			if u.Host != "" {
				endpoint.Host = u.Host
			}

			endpoints = append(endpoints, endpoint)
		}
	}

	cancelCtx, cancelFn := context.WithCancel(context.Background())

	forwarder := &TelemetryForwarder{
		endpoints: endpoints,
		conf:      conf,

		forwardedReqChan: make(chan forwardedRequest, maxInflightRequests-maxConcurrentRequests),
		inflightWaiter:   sync.WaitGroup{},
		inflightCount:    atomic.Int64{},
		maxInflightBytes: maxInflightBytes,

		cancelCtx: cancelCtx,
		cancelFn:  cancelFn,
		done:      make(chan struct{}),

		containerIDProvider: containerIDProvider,
		client:              conf.NewHTTPClient(),
		statsd:              statsd,
		logger:              log.NewThrottled(5, 10*time.Second),
		cmdLineScrubber:     newCmdLineScrubber(),
	}
	return forwarder
}

func (f *TelemetryForwarder) start() {
	for i := 0; i < maxConcurrentRequests; i++ {
		f.inflightWaiter.Add(1)
		go func() {
			defer f.inflightWaiter.Done()
			for {
				select {
				case <-f.done:
					return
				case req, ok := <-f.forwardedReqChan:
					if !ok {
						return
					}
					f.forwardTelemetry(req)
				}
			}
		}()
	}
}

type forwardedRequest struct {
	req  *http.Request
	body []byte
}

// Stop waits for up to 1s to end all telemetry forwarded requests.
func (f *TelemetryForwarder) Stop() {
	close(f.done)
	done := make(chan any)
	go func() {
		f.inflightWaiter.Wait()
		close(done)
	}()
	select {
	case <-done:
	// Give a max 1s timeout to wait for all requests to end
	case <-time.After(1 * time.Second):
	}
	f.cancelFn()
}

func (f *TelemetryForwarder) startRequest(size int64) (accepted bool) {
	for {
		inflight := f.inflightCount.Load()
		newInflight := inflight + size
		if newInflight > f.maxInflightBytes {
			return false
		}
		if f.inflightCount.CompareAndSwap(inflight, newInflight) {
			return true
		}
	}
}

func (f *TelemetryForwarder) endRequest(req forwardedRequest) {
	f.inflightCount.Add(-int64(len(req.body)))
	req.body = nil
}

// telemetryForwarderHandler returns a new HTTP handler which will proxy requests to the configured intakes.
// If the main intake URL can not be computed because of config, the returned handler will always
// return http.StatusInternalServerError along with a clarification.
//
// This proxying will happen asynchronously and the handler will respond automatically. To still have backpressure
// we will respond with StatusTooManyRequests if we have to many request being forwarded concurrently.
func (r *HTTPReceiver) telemetryForwarderHandler() http.Handler {
	if len(r.telemetryForwarder.endpoints) == 0 {
		log.Error("None of the configured apm_config.telemetry endpoints are valid. Telemetry proxy is off")
		return http.NotFoundHandler()
	}

	forwarder := r.telemetryForwarder
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Read at most maxInflightBytes since we're going to throw out the result anyway if it's bigger
		body, err := io.ReadAll(io.LimitReader(r.Body, forwarder.maxInflightBytes+1))
		if err != nil {
			writeEmptyJSON(w, http.StatusInternalServerError)
			return
		}

		body = forwarder.stripCommandLineSecrets(r, body)

		if accepted := forwarder.startRequest(int64(len(body))); !accepted {
			writeEmptyJSON(w, http.StatusTooManyRequests)
			return
		}

		newReq, err := http.NewRequestWithContext(forwarder.cancelCtx, r.Method, r.URL.String(), bytes.NewBuffer(body))
		if err != nil {
			writeEmptyJSON(w, http.StatusInternalServerError)
			return
		}
		newReq.Header = r.Header.Clone()
		select {
		case forwarder.forwardedReqChan <- forwardedRequest{
			req:  newReq,
			body: body,
		}:
			writeEmptyJSON(w, http.StatusOK)
		default:
			writeEmptyJSON(w, http.StatusTooManyRequests)
		}
	})
}

// scrubCommandLine redacts secrets from a raw command line string, covering
// both "--password=hunter2"/"password: hunter2" forms (pkg/util/scrubber's
// defaults) and the space-delimited "--password hunter2" form (added by
// f.cmdLineScrubber above). It cannot redact a secret passed as a bare
// positional argument with no recognizable flag name (e.g. "mysql root
// hunter2").
func (f *TelemetryForwarder) scrubCommandLine(cmdLine string) string {
	return f.cmdLineScrubber.ScrubLine(cmdLine)
}

func (f *TelemetryForwarder) stripCommandLineSecrets(req *http.Request, body []byte) []byte {
	if req.Header.Get(telemetryRequestTypeHeader) != apmTelemetryRequestType {
		return body
	}
	if req.URL.Path != apmTelemetryProxyPath {
		return body
	}

	var msg telemetryRequest
	if err := json.Unmarshal(body, &msg); err != nil {
		f.logger.Error("telemetry proxy: failed to decode injection-metadata envelope: %v", err)
		return body
	}
	if len(msg.Payload) == 0 {
		return body
	}

	var payload injectionMetadata
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		f.logger.Error("telemetry proxy: failed to decode injection-metadata payload: %v", err)
		return body
	}

	rawPayload := msg.Payload
	changed := false

	if payload.CommandLine != "" {
		scrubbed := f.scrubCommandLine(payload.CommandLine)
		if scrubbed != payload.CommandLine {
			scrubbedJSON, err := json.Marshal(scrubbed)
			if err != nil {
				f.logger.Error("telemetry proxy: failed to encode scrubbed command_line: %v", err)
				return body
			}
			rawPayload, err = patchJSONField(rawPayload, "command_line", scrubbedJSON)
			if err != nil {
				f.logger.Error("telemetry proxy: failed to re-encode injection-metadata payload: %v", err)
				return body
			}
			changed = true
		}
	}

	if len(payload.Metadata) > 0 {
		scrubbedMetadata, metadataChanged, err := scrubJSONValue(payload.Metadata, f.cmdLineScrubber)
		if err != nil {
			f.logger.Error("telemetry proxy: failed to scrub injection-metadata metadata field: %v", err)
			return body
		}
		if metadataChanged {
			rawPayload, err = patchJSONField(rawPayload, "metadata", scrubbedMetadata)
			if err != nil {
				f.logger.Error("telemetry proxy: failed to re-encode injection-metadata payload: %v", err)
				return body
			}
			changed = true
		}
	}

	if !changed {
		return body
	}

	out, err := patchJSONField(body, "payload", rawPayload)
	if err != nil {
		f.logger.Error("telemetry proxy: failed to re-encode injection-metadata envelope: %v", err)
		return body
	}
	return out
}

// isSensitiveMetadataKey matches JSON object keys within the free-form
// injection-metadata metadata field against sensitiveArgFlags, since its
// shape isn't fixed and may carry a raw property value directly under its
// property/env-var name (e.g. {"DD_API_KEY": "..."}) rather than a
// "flag=value" pair scrubCommandLine's regexes can key off of. It matches by
// substring, like isSensitiveWord, so prefixed/suffixed names such as
// "DD_API_KEY" or "AUTH_TOKEN" are still caught.
func isSensitiveMetadataKey(key string) bool {
	return isSensitiveWord(key)
}

var valueKeyNames = map[string]bool{"value": true, "val": true}

func isSensitiveWord(s string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(s, "-", "_"))
	for _, word := range sensitiveArgFlags {
		if strings.Contains(normalized, word) {
			return true
		}
	}
	return false
}

// scrubJSONValue walks an arbitrary JSON value and redacts string leaves: outright, when the enclosing
// object key names a known-sensitive field (see isSensitiveMetadataKey) or
// when a sibling key's value names one (see valueKeyNames/isSensitiveWord),
// and otherwise via s, which redacts embedded "flag=value"/"flag: value"
// patterns the same way it does for command_line. It reports whether
// anything changed so callers can skip re-encoding untouched payloads.
func scrubJSONValue(raw json.RawMessage, s *scrubber.Scrubber) (json.RawMessage, bool, error) {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw, false, err
	}
	scrubbed, changed := scrubValue("", v, s)
	if !changed {
		return raw, false, nil
	}
	out, err := json.Marshal(scrubbed)
	if err != nil {
		return raw, false, err
	}
	return out, true, nil
}

func scrubValue(key string, v interface{}, s *scrubber.Scrubber) (interface{}, bool) {
	switch val := v.(type) {
	case string:
		if isSensitiveMetadataKey(key) {
			if val == "********" {
				return val, false
			}
			return "********", true
		}
		scrubbed := s.ScrubLine(val)
		return scrubbed, scrubbed != val
	case map[string]interface{}:
		hasSensitiveNameDesignator := false
		for k, elem := range val {
			if str, ok := elem.(string); ok && !valueKeyNames[strings.ToLower(k)] && isSensitiveWord(str) {
				hasSensitiveNameDesignator = true
				break
			}
		}
		changed := false
		for k, elem := range val {
			if hasSensitiveNameDesignator && valueKeyNames[strings.ToLower(k)] {
				if str, ok := elem.(string); ok {
					if str != "********" {
						val[k] = "********"
						changed = true
					}
					continue
				}
			}
			scrubbedElem, elemChanged := scrubValue(k, elem, s)
			if elemChanged {
				val[k] = scrubbedElem
				changed = true
			}
		}
		return val, changed
	case []interface{}:
		changed := false
		for i, elem := range val {
			scrubbedElem, elemChanged := scrubValue(key, elem, s)
			if elemChanged {
				val[i] = scrubbedElem
				changed = true
			}
		}
		return val, changed
	default:
		return v, false
	}
}

func writeEmptyJSON(w http.ResponseWriter, statusCode int) {
	w.WriteHeader(statusCode)
	w.Write([]byte("{}"))
}

func (f *TelemetryForwarder) setRequestHeader(req *http.Request) {
	req.Header.Set("Via", "trace-agent "+f.conf.AgentVersion)
	if _, ok := req.Header["User-Agent"]; !ok {
		// explicitly disable User-Agent so it's not set to the default value
		// that net/http gives it: Go-http-client/1.1
		// See https://codereview.appspot.com/7532043
		req.Header.Set("User-Agent", "")
	}

	containerID := f.containerIDProvider.GetContainerID(req.Context(), req.Header)
	if containerID == "" {
		_ = f.statsd.Count("datadog.trace_agent.telemetry_proxy.no_container_id_found", 1, []string{}, 1)
	}
	containerTags := getContainerTags(f.conf.ContainerTags, containerID)

	req.Header.Set("DD-Agent-Hostname", f.conf.Hostname)
	req.Header.Set("DD-Agent-Env", f.conf.DefaultEnv)
	log.Debugf("Setting headers DD-Agent-Hostname=%s, DD-Agent-Env=%s for telemetry proxy", f.conf.Hostname, f.conf.DefaultEnv)
	if containerID != "" {
		req.Header.Set(header.ContainerID, containerID)
	}
	if containerTags != "" {
		ctagsHeader := normalizeHTTPHeader(containerTags)
		req.Header.Set("X-Datadog-Container-Tags", ctagsHeader)
		log.Debugf("Setting header X-Datadog-Container-Tags=%s for telemetry proxy", ctagsHeader)
	}
	if f.conf.InstallSignature.Found {
		req.Header.Set("DD-Agent-Install-Id", f.conf.InstallSignature.InstallID)
		req.Header.Set("DD-Agent-Install-Type", f.conf.InstallSignature.InstallType)
		req.Header.Set("DD-Agent-Install-Time", strconv.FormatInt(f.conf.InstallSignature.InstallTime, 10))
	}
	if taskArn, ok := extractFargateTask(containerTags); ok {
		req.Header.Set(cloudProviderHeader, string(aws))
		req.Header.Set(cloudResourceTypeHeader, string(awsFargate))
		req.Header.Set(cloudResourceIdentifierHeader, taskArn)
	}
	if origin, ok := f.conf.GlobalTags[originTag]; ok {
		switch origin {
		case "cloudrun":
			req.Header.Set(cloudProviderHeader, string(gcp))
			req.Header.Set(cloudResourceTypeHeader, string(cloudRun))
			if serviceName, found := f.conf.GlobalTags["service_name"]; found {
				req.Header.Set(cloudResourceIdentifierHeader, serviceName)
			}
		case "cloudfunction":
			req.Header.Set(cloudProviderHeader, string(gcp))
			req.Header.Set(cloudResourceTypeHeader, string(cloudFunctions))
			if serviceName, found := f.conf.GlobalTags["service_name"]; found {
				req.Header.Set(cloudResourceIdentifierHeader, serviceName)
			}
		case "appservice":
			req.Header.Set(cloudProviderHeader, string(azure))
			req.Header.Set(cloudResourceTypeHeader, string(azureAppService))
			if appName, found := f.conf.GlobalTags["app_name"]; found {
				req.Header.Set(cloudResourceIdentifierHeader, appName)
			}
		case "containerapp":
			req.Header.Set(cloudProviderHeader, string(azure))
			req.Header.Set(cloudResourceTypeHeader, string(azureContainerApp))
			if appName, found := f.conf.GlobalTags["app_name"]; found {
				req.Header.Set(cloudResourceIdentifierHeader, appName)
			}
		}
	}
}

// forwardTelemetry sends request first to Endpoint[0], then sends a copy of main request to every configurged
// additional endpoint.
//
// All requests will be sent irregardless of any errors
// If any request fails, the error will be logged.
func (f *TelemetryForwarder) forwardTelemetry(req forwardedRequest) {
	defer f.endRequest(req)

	f.setRequestHeader(req.req)

	for i, e := range f.endpoints {
		var newReq *http.Request
		if i != len(f.endpoints)-1 {
			newReq = req.req.Clone(req.req.Context())
		} else {
			// don't clone the request for the last endpoint since we can use the
			// one provided in args.
			newReq = req.req
		}
		newReq.Body = io.NopCloser(bytes.NewReader(req.body))

		if resp, err := f.forwardTelemetryEndpoint(newReq, e); err == nil {
			if !(200 <= resp.StatusCode && resp.StatusCode < 300) {
				f.logger.Error("Received unexpected status code %v", resp.StatusCode)
			}
			io.Copy(io.Discard, resp.Body) // nolint:errcheck
			resp.Body.Close()
		} else {
			f.logger.Error("%v", err)
		}
	}
}

func (f *TelemetryForwarder) forwardTelemetryEndpoint(req *http.Request, endpoint *config.Endpoint) (*http.Response, error) {
	tags := []string{
		"endpoint:" + endpoint.Host,
	}
	defer func(now time.Time) {
		_ = f.statsd.Timing("datadog.trace_agent.telemetry_proxy.roundtrip_ms", time.Since(now), tags, 1)
	}(time.Now())

	req.Host = endpoint.Host
	req.URL.Host = endpoint.Host
	req.URL.Scheme = "https"
	req.Header.Set("DD-API-KEY", endpoint.APIKey)

	resp, err := f.client.Do(req)
	if err != nil {
		_ = f.statsd.Count("datadog.trace_agent.telemetry_proxy.error", 1, tags, 1)
	}
	return resp, err
}

func extractFargateTask(containerTags string) (string, bool) {
	return extractTag(containerTags, "task_arn")
}

func extractTag(tags string, name string) (string, bool) {
	leftoverTags := tags
	for {
		if leftoverTags == "" {
			return "", false
		}
		var tag string
		tag, leftoverTags, _ = strings.Cut(leftoverTags, ",")

		tagName, value, hasValue := strings.Cut(tag, ":")
		if hasValue && tagName == name {
			return value, true
		}
	}
}

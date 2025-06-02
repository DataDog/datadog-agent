// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-go/v5/statsd"
)

const functionARNKeyTag = "function_arn"
const originTag = "origin"

type cloudResourceType string
type cloudProvider string

const (
	awsLambda                     cloudResourceType = "AWSLambda"
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

func writeEmptyJSON(w http.ResponseWriter, statusCode int) {
	w.WriteHeader(statusCode)
	w.Write([]byte("{}"))
}

func (f *TelemetryForwarder) setRequestHeader(req *http.Request) {
	req.Header.Set("Via", fmt.Sprintf("trace-agent %s", f.conf.AgentVersion))
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
	if arn, ok := f.conf.GlobalTags[functionARNKeyTag]; ok {
		req.Header.Set(cloudProviderHeader, string(aws))
		req.Header.Set(cloudResourceTypeHeader, string(awsLambda))
		req.Header.Set(cloudResourceIdentifierHeader, arn)
	} else if taskArn, ok := extractFargateTask(containerTags); ok {
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
		fmt.Sprintf("endpoint:%s", endpoint.Host),
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

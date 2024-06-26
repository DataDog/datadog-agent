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
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"

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
	azureAppService               cloudResourceType = "AzureAppService"
	azureContainerApp             cloudResourceType = "AzureContainerApp"
	aws                           cloudProvider     = "AWS"
	gcp                           cloudProvider     = "GCP"
	azure                         cloudProvider     = "Azure"
	cloudProviderHeader           string            = "dd-cloud-provider"
	cloudResourceTypeHeader       string            = "dd-cloud-resource-type"
	cloudResourceIdentifierHeader string            = "dd-cloud-resource-identifier"
)

// TelemetryForwarder sends HTTP requests to multiple targets.
// The handler returns immediately and the forwarding is done in the background.
//
// To provide somne backpressure, we limit the number of concurrent forwarded requests
type TelemetryForwarder struct {
	endpoints []*config.Endpoint
	conf      *config.AgentConfig

	inflightWaiter sync.WaitGroup
	inflightCount  atomic.Int32
	maxConcurrent  int32

	containerIDProvider IDProvider
	client              *config.ResetClient
	statsd              statsd.ClientInterface
	logger              *log.ThrottledLogger
}

// NewTelemetryForwarder creates a new TelemetryForwarder
func NewTelemetryForwarder(conf *config.AgentConfig, containerIDProvider IDProvider, statsd statsd.ClientInterface) *TelemetryForwarder {
	// extract and validate Hostnames from configured endpoints
	var endpoints []*config.Endpoint
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

	return &TelemetryForwarder{
		endpoints: endpoints,
		conf:      conf,

		inflightWaiter: sync.WaitGroup{},
		inflightCount:  atomic.Int32{},
		maxConcurrent:  100,

		containerIDProvider: containerIDProvider,
		client:              conf.NewHTTPClient(),
		statsd:              statsd,
		logger:              log.NewThrottled(5, 10*time.Second),
	}
}

type Request struct {
	req  *http.Request
	body []byte
}

// Stop waits for up to 1s to end all telemetry forwarded requests.
func (f *TelemetryForwarder) Stop() {
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
}

func (f *TelemetryForwarder) startRequest() (accepted bool) {
	for {
		inflight := f.inflightCount.Load()
		if inflight >= f.maxConcurrent {
			return false
		}
		if f.inflightCount.CompareAndSwap(inflight, inflight+1) {
			return true
		}
	}
}

func (f *TelemetryForwarder) endRequest() {
	f.inflightCount.Add(-1)
}

func (f *TelemetryForwarder) forwardTelemetryAsynchronously(r *http.Request) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		f.endRequest()
		return err
	}
	req := Request{
		req:  r.Clone(context.Background()),
		body: body,
	}

	f.inflightWaiter.Add(1)
	go func() {
		defer f.inflightWaiter.Done()
		defer f.endRequest()

		f.setRequestHeader(req.req)
		f.forwardTelemetry(context.Background(), req)
	}()
	return nil
}

// telemetryForwarderHandler returns a new HTTP handler which will proxy requests to the configured intakes.
// If the main intake URL can not be computed because of config, the returned handler will always
// return http.StatusInternalServerError along with a clarification.
//
// This proxying will happen asynchronously and the handler will respond automatically. To still have backpressure
// we will responf with 429 if we have to many request being forwarded concurrently.
func (r *HTTPReceiver) telemetryForwarderHandler() http.Handler {
	if len(r.telemetryForwarder.endpoints) == 0 {
		log.Error("None of the configured apm_config.telemetry endpoints are valid. Telemetry proxy is off")
		return http.NotFoundHandler()
	}

	forwarder := r.telemetryForwarder
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if accepted := forwarder.startRequest(); !accepted {
			writeEmptyJson(w, 429)
			return
		}
		err := forwarder.forwardTelemetryAsynchronously(r)
		if err != nil {
			writeEmptyJson(w, 400)
			return
		}
		writeEmptyJson(w, 200)
	})
}

func writeEmptyJson(w http.ResponseWriter, statusCode int) {
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
		req.Header.Set("x-datadog-container-tags", containerTags)
		log.Debugf("Setting header x-datadog-container-tags=%s for telemetry proxy", containerTags)
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
func (f *TelemetryForwarder) forwardTelemetry(ctx context.Context, req Request) {
	for i, e := range f.endpoints {
		var newReq *http.Request
		if i != len(f.endpoints)-1 {
			newReq = req.req.Clone(ctx)
		} else {
			// don't clone the request for the last endpoint since we can use the
			// one provided in args.
			newReq = req.req
		}
		newReq.Body = io.NopCloser(bytes.NewReader(req.body))

		if resp, err := f.forwardTelemetryEndpoint(newReq, e); err == nil {
			io.Copy(io.Discard, resp.Body)
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

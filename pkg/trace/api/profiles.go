// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"fmt"
	"io"
	stdlog "log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	// profilingURLPath specifies the default intake API path
	profilingURLPath = "/api/v2/profile"
	// profilingV1EndpointSuffix suffix identifying a user-configured V1 endpoint
	profilingV1EndpointSuffix = "v1/input"
)

// endpointDescriptor specifies the configuration of an endpoint for profiling data.
type endpointDescriptor struct {
	// url specifies the URL to send profiles too.
	url *url.URL
	// apiKey specifies the Datadog API key to use.
	apiKey string
	// isEnabled is a function that can be called to understand if this endpoint should be used
	// one can use this to disable endpoints dynamically (e.g. due to multi_region_failover)
	isEnabled func() bool
}

// profilingEndpoints returns the profiling intake urls and their corresponding
// api keys based on agent configuration. The main endpoint is always returned as
// the first element in the slice.
func profilingEndpoints(conf *config.AgentConfig) (endpoints []endpointDescriptor, err error) {
	for i, endpoint := range conf.ProfilingProxy.Endpoints {
		url, err := url.Parse(endpoint.Host)
		if err != nil {
			if i == 0 {
				// main endpoint parsing failure is fatal
				return nil, fmt.Errorf("Error parsing main profiling intake URL: %s: %v", endpoint.Host, err)
			}

			log.Errorf("Error parsing additional profiling intake URL %s: %v", endpoint.Host, err)
			continue
		}

		if len(url.Path) > 0 {
			if url.Path == profilingV1EndpointSuffix {
				log.Warnf("The configured url %s for apm_config.profiling_dd_url is deprecated. "+
					"The updated endpoint path is /api/v2/profile and this is the default path used if none is specified.", url.String())
			}
		} else {
			url.Path = profilingURLPath
		}

		endpoints = append(endpoints, endpointDescriptor{
			url:    url,
			apiKey: endpoint.APIKey,
			isEnabled: func() bool {
				if !endpoint.IsMRF {
					return true
				}

				return conf.MRFFailoverProfiling()
			},
		})
	}
	return
}

// profileProxyHandler returns a new HTTP handler which will proxy requests to the profiling intakes.
// If the main intake URL can not be computed because of config, the returned handler will always
// return http.StatusInternalServerError along with a clarification.
func (r *HTTPReceiver) profileProxyHandler() http.Handler {
	endpoints, err := profilingEndpoints(r.conf)
	if err != nil {
		return errorHandler(err)
	}
	var tags strings.Builder
	tags.WriteString(fmt.Sprintf("host:%s,default_env:%s,agent_version:%s", r.conf.Hostname, r.conf.DefaultEnv, r.conf.AgentVersion))

	if orch := r.conf.FargateOrchestrator; orch != config.OrchestratorUnknown {
		tags.WriteString(fmt.Sprintf(",orchestrator:fargate_%s", strings.ToLower(string(orch))))
	}
	if r.conf.LambdaFunctionName != "" {
		tags.WriteString(fmt.Sprintf("functionname:%s", strings.ToLower(r.conf.LambdaFunctionName)))
		tags.WriteString("_dd.origin:lambda")
	}
	if r.conf.AzureContainerAppTags != "" {
		tags.WriteString(r.conf.AzureContainerAppTags)
	}

	return newProfileProxy(r.conf, endpoints, tags.String(), r.statsd)
}

func errorHandler(err error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		msg := fmt.Sprintf("Profile forwarder is OFF: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
	})
}

// newProfileProxy creates an http.ReverseProxy which can forward requests to
// one or more endpoints.
//
// The endpoint URLs are passed in through the targets slice. Each endpoint
// must have a corresponding API key in the same position in the keys slice.
//
// The tags will be added as a header to all proxied requests.
// For more details please see multiTransport.
func newProfileProxy(conf *config.AgentConfig, endpoints []endpointDescriptor, tags string, statsd statsd.ClientInterface) *httputil.ReverseProxy {
	cidProvider := NewIDProvider(conf.ContainerProcRoot, conf.ContainerIDFromOriginInfo)
	director := func(req *http.Request) {
		req.Header.Set("Via", fmt.Sprintf("trace-agent %s", conf.AgentVersion))
		if _, ok := req.Header["User-Agent"]; !ok {
			// explicitly disable User-Agent so it's not set to the default value
			// that net/http gives it: Go-http-client/1.1
			// See https://codereview.appspot.com/7532043
			req.Header.Set("User-Agent", "")
		}
		containerID := cidProvider.GetContainerID(req.Context(), req.Header)
		if ctags := getContainerTags(conf.ContainerTags, containerID); ctags != "" {
			ctagsHeader := normalizeHTTPHeader(ctags)
			req.Header.Set("X-Datadog-Container-Tags", ctagsHeader)
			log.Debugf("Setting header X-Datadog-Container-Tags=%s for profiles proxy", ctagsHeader)
		}
		req.Header.Set("X-Datadog-Additional-Tags", tags)
		log.Debugf("Setting header X-Datadog-Additional-Tags=%s for profiles proxy", tags)
		_ = statsd.Count("datadog.trace_agent.profile", 1, nil, 1)
		// URL, Host and key are set in the transport for each outbound request
	}
	transport := conf.NewHTTPTransport()
	// The intake's connection timeout is 60 seconds, which is similar to the default profiling periodicity of our
	// tracers. When a new profile upload is simultaneous to the intake closing the connection, Go's ReverseProxy
	// returns a 502 error to the tracer. Ensuring that the agent closes the connection before the intake solves this
	// race condition. A value of 47 was chosen as it's a prime number which doesn't divide 60, reducing the risk of
	// overlap with other timeouts or periodicities. It provides sufficient buffer time compared to 60, whilst still
	// allowing connection reuse for tracer setups that upload multiple profiles per minute.
	transport.IdleConnTimeout = 47 * time.Second
	logger := log.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	return &httputil.ReverseProxy{
		Director:  director,
		ErrorLog:  stdlog.New(logger, "profiling.Proxy: ", 0),
		Transport: &multiTransport{transport, endpoints},
	}
}

// multiTransport sends HTTP requests to multiple targets using an
// underlying http.RoundTripper. API keys are set separately for each target.
// When multiple endpoints are in use the response from the main endpoint
// is proxied back to the client, while for all aditional endpoints the
// response is discarded. There is no de-duplication done between endpoint
// hosts or api keys.
type multiTransport struct {
	rt        http.RoundTripper
	endpoints []endpointDescriptor
}

func (m *multiTransport) RoundTrip(req *http.Request) (rresp *http.Response, rerr error) {
	setTarget := func(r *http.Request, u *url.URL, apiKey string) {
		r.Host = u.Host
		r.URL = u
		r.Header.Set("DD-API-KEY", apiKey)
	}
	defer func() {
		// Hack for backwards-compatibility
		// The old v1/input endpoint responded with 200 and as this handler
		// is just a proxy to existing clients, some clients break on
		// encountering a 202 response when proxying for the new api/v2/profile endpoints.
		if rresp != nil && rresp.StatusCode == http.StatusAccepted {
			rresp.Status = http.StatusText(http.StatusOK)
			rresp.StatusCode = http.StatusOK
		}
	}()
	if len(m.endpoints) == 1 {
		setTarget(req, m.endpoints[0].url, m.endpoints[0].apiKey)
		return m.rt.RoundTrip(req)
	}
	slurp, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	for i, e := range m.endpoints {
		newreq := req.Clone(req.Context())
		newreq.Body = io.NopCloser(bytes.NewReader(slurp))
		setTarget(newreq, e.url, e.apiKey)
		if i == 0 {
			// given the way we construct the list of targets the main endpoint
			// will be the first one called, we return its response and error
			rresp, rerr = m.rt.RoundTrip(newreq)
			continue
		}

		if resp, err := m.rt.RoundTrip(newreq); err == nil {
			// we discard responses for all subsequent requests
			io.Copy(io.Discard, resp.Body) //nolint:errcheck
			resp.Body.Close()
		} else {
			log.Error(err)
		}
	}
	return rresp, rerr
}

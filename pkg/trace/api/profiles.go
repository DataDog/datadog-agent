// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	// profilingURLTemplate specifies the template for obtaining the profiling URL along with the site.
	profilingURLTemplate = "https://intake.profile.%s/api/v2/profile"
	// profilingURLDefault specifies the default intake API URL.
	profilingURLDefault = "https://intake.profile.datadoghq.com/api/v2/profile"
	// profilingV1EndpointSuffix suffix identifying a user-configured V1 endpoint
	profilingV1EndpointSuffix = "v1/input"
)

// profilingEndpoints returns the profiling intake urls and their corresponding
// api keys based on agent configuration. The main endpoint is always returned as
// the first element in the slice.
func profilingEndpoints(conf *config.AgentConfig) (urls []*url.URL, apiKeys []string, err error) {
	main := profilingURLDefault
	if v := conf.ProfilingProxy.DDURL; v != "" {
		main = v
		if strings.HasSuffix(main, profilingV1EndpointSuffix) {
			log.Warnf("The configured url %s for apm_config.profiling_dd_url is deprecated. "+
				"The updated endpoint path is /api/v2/profile.", v)
		}
	} else if conf.Site != "" {
		main = fmt.Sprintf(profilingURLTemplate, conf.Site)
	}
	u, err := url.Parse(main)
	if err != nil {
		// if the main intake URL is invalid we don't use additional endpoints
		return nil, nil, fmt.Errorf("error parsing main profiling intake URL %s: %v", main, err)
	}
	urls = append(urls, u)
	apiKeys = append(apiKeys, conf.APIKey())

	if extra := conf.ProfilingProxy.AdditionalEndpoints; extra != nil {
		for endpoint, keys := range extra {
			u, err := url.Parse(endpoint)
			if err != nil {
				log.Errorf("Error parsing additional profiling intake URL %s: %v", endpoint, err)
				continue
			}
			for _, key := range keys {
				urls = append(urls, u)
				apiKeys = append(apiKeys, key)
			}
		}
	}
	return urls, apiKeys, nil
}

// profileProxyHandler returns a new HTTP handler which will proxy requests to the profiling intakes.
// If the main intake URL can not be computed because of config, the returned handler will always
// return http.StatusInternalServerError along with a clarification.
func (r *HTTPReceiver) profileProxyHandler() http.Handler {
	targets, keys, err := profilingEndpoints(r.conf)
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
	if r.conf.AzureServerlessTags != "" {
		tags.WriteString(r.conf.AzureServerlessTags)
	}

	return newProfileProxy(r.conf, targets, keys, tags.String(), r.statsd)
}

func errorHandler(err error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		msg := fmt.Sprintf("Profile forwarder is OFF: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
	})
}

// isRetryableBodyReadError determines if a body read error should be retried
func isRetryableBodyReadError(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific connection errors during body read
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "read" {
			return true
		}
	}

	// Check for network-level errors that might be transient
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	// Check for context cancellation (might be transient)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// Default to false, this covers EOF and other stream-related errors
	return false
}

// newProfileProxy creates an http.ReverseProxy which can forward requests to
// one or more endpoints.
//
// The endpoint URLs are passed in through the targets slice. Each endpoint
// must have a corresponding API key in the same position in the keys slice.
//
// The tags will be added as a header to all proxied requests.
// For more details please see multiTransport.
func newProfileProxy(conf *config.AgentConfig, targets []*url.URL, keys []string, tags string, statsd statsd.ClientInterface) *httputil.ReverseProxy {
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
	ptransport := newProfilingTransport(transport)
	logger := log.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	return &httputil.ReverseProxy{
		Director:     director,
		ErrorLog:     stdlog.New(logger, "profiling.Proxy: ", 0),
		Transport:    &multiTransport{ptransport, targets, keys},
		ErrorHandler: handleProxyError,
	}
}

// handleProxyError handles errors from the profiling reverse proxy with appropriate
// HTTP status codes and comprehensive logging.
func handleProxyError(w http.ResponseWriter, r *http.Request, err error) {
	// Extract useful context for logging
	var payloadSize int64
	if r.ContentLength > 0 {
		payloadSize = r.ContentLength
	} else if r.Body != nil {
		// For chunked uploads, ContentLength is 0 but there might still be a body
		// Try to get a size estimate, but don't consume the body as it may have already been read
		payloadSize = -1 // Indicate chunked/unknown size
	}

	var timeoutSetting time.Duration
	if deadline, ok := r.Context().Deadline(); ok {
		timeoutSetting = time.Until(deadline)
	}

	// Determine appropriate HTTP status code based on error type
	var statusCode int
	var errorType string

	if errors.Is(err, context.DeadlineExceeded) {
		// Context deadline exceeded during request processing
		// This typically means the client was too slow uploading the request body
		statusCode = http.StatusRequestTimeout // 408 (client timeout)
		errorType = "request timeout"
	} else if isRetryableBodyReadError(err) {
		// Check if this is specifically a timeout error
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			statusCode = http.StatusRequestTimeout // 408 (client timeout)
			errorType = "body read timeout"
		} else {
			statusCode = http.StatusServiceUnavailable // 503 (other retryable errors)
			errorType = "retryable body read error"
		}
	} else { // default case
		statusCode = http.StatusBadGateway // 502 (default)
		errorType = "transport error"
	}

	// Single comprehensive log with all context
	if payloadSize == -1 {
		log.Warnf("profiling proxy error: %s (%d) for %s %s - payload_size=chunked timeout_remaining=%v error=%v",
			errorType, statusCode, r.Method, r.URL.Path, timeoutSetting, err)
	} else {
		log.Warnf("profiling proxy error: %s (%d) for %s %s - payload_size=%d timeout_remaining=%v error=%v",
			errorType, statusCode, r.Method, r.URL.Path, payloadSize, timeoutSetting, err)
	}

	w.WriteHeader(statusCode)
	if _, writeErr := w.Write([]byte(err.Error())); writeErr != nil {
		log.Debugf("Failed to write error response body: %v", writeErr)
	}
}

// multiTransport sends HTTP requests to multiple targets using an
// underlying http.RoundTripper. API keys are set separately for each target.
// When multiple endpoints are in use the response from the main endpoint
// is proxied back to the client, while for all aditional endpoints the
// response is discarded. There is no de-duplication done between endpoint
// hosts or api keys.
type multiTransport struct {
	rt      http.RoundTripper
	targets []*url.URL
	keys    []string
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
	if len(m.targets) == 1 {
		setTarget(req, m.targets[0], m.keys[0])
		rresp, rerr = m.rt.RoundTrip(req)
		// Avoid sub-sequent requests from getting a use of closed network connection error
		if rerr != nil && req.Body != nil {
			req.Body.Close()
		}
		return rresp, rerr
	}
	slurp, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	for i, u := range m.targets {
		newreq := req.Clone(req.Context())
		newreq.Body = io.NopCloser(bytes.NewReader(slurp))
		setTarget(newreq, u, m.keys[i])
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

// profilingTransport wraps an *http.Transport to improve connection hygiene after
// response body copy errors by flushing idle connections and forcing the next
// outbound request to close instead of reusing a possibly bad connection.
type profilingTransport struct {
	*http.Transport
	forceCloseNext atomic.Bool
}

func newProfilingTransport(transport *http.Transport) *profilingTransport {
	return &profilingTransport{Transport: transport}
}

func (p *profilingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// If a previous response body read error occurred, force this request to not reuse connections.
	if p.forceCloseNext.Load() {
		// Clone to avoid mutating caller's request.
		req = req.Clone(req.Context())
		req.Close = true
		req.Header.Set("Connection", "close")
		p.forceCloseNext.Store(false)
	}

	resp, err := p.Transport.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}

	// Wrap the response body to detect mid-stream read errors during reverse proxy copy.
	origBody := resp.Body
	resp.Body = &readTrackingBody{
		ReadCloser: origBody,
		onReadError: func(rerr error) {
			if rerr != nil && rerr != io.EOF {
				p.CloseIdleConnections()
				p.forceCloseNext.Store(true)
				log.Warnf("profiling proxy: upstream body read error detected, flushed idle conns and will force close on next request: %v", rerr)
			}
		},
	}
	return resp, nil
}

type readTrackingBody struct {
	io.ReadCloser
	onReadError func(error)
}

func (b *readTrackingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && err != io.EOF && b.onReadError != nil {
		b.onReadError(err)
	}
	return n, err
}

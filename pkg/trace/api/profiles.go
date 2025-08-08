// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"syscall"
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

	// Retry configuration
	maxRetryAttempts = 3
	retryBaseDelay   = 100 * time.Millisecond
	retryMaxDelay    = 2 * time.Second
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
	if r.conf.AzureContainerAppTags != "" {
		tags.WriteString(r.conf.AzureContainerAppTags)
	}

	return newProfileProxy(r.conf, targets, keys, tags.String(), r.statsd)
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
	logger := log.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	return &httputil.ReverseProxy{
		Director:  director,
		ErrorLog:  stdlog.New(logger, "profiling.Proxy: ", 0),
		Transport: &multiTransport{transport, targets, keys},
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

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for network-level errors that indicate connection issues
	var netErr net.Error
	if errors.As(err, &netErr) {
		// Retry on timeout or temporary network errors
		return netErr.Timeout() || netErr.Temporary()
	}

	// Check for specific connection errors
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "dial" || opErr.Op == "read" || opErr.Op == "write" {
			return true
		}
		if syscallErr, ok := opErr.Err.(*net.DNSError); ok {
			return syscallErr.Temporary()
		}
		if syscallErr, ok := opErr.Err.(*syscall.Errno); ok {
			// Common retryable syscall errors
			switch *syscallErr {
			case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ETIMEDOUT, syscall.ENETUNREACH, syscall.EHOSTUNREACH:
				return true
			}
		}
	}

	return false
}

// isRetryableStatusCode determines if an HTTP status code should trigger a retry
func isRetryableStatusCode(statusCode int) bool {
	switch statusCode {
	case http.StatusBadGateway, // 502 - proxy/gateway connection issues
		http.StatusServiceUnavailable, // 503 - temporary server overload
		http.StatusGatewayTimeout:     // 504 - proxy/gateway timeout
		return true
	default:
		return false
	}
}

// calculateRetryDelay computes the delay for a retry attempt using exponential backoff
func calculateRetryDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return retryBaseDelay
	}

	delay := retryBaseDelay
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay > retryMaxDelay {
			return retryMaxDelay
		}
	}
	return delay
}

func (m *multiTransport) RoundTrip(req *http.Request) (rresp *http.Response, rerr error) {
	setTarget := func(r *http.Request, u *url.URL, apiKey string) {
		r.Host = u.Host
		r.URL = u
		r.Header.Set("DD-API-KEY", apiKey)
	}

	// Read the request body once for all retry attempts and multiple targets
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body.Close() // Close the original body
	}

	// retryRoundTrip performs a single request with retry logic using pre-read body bytes
	retryRoundTrip := func(u *url.URL, apiKey string) (*http.Response, error) {
		var lastErr error
		var resp *http.Response

		for attempt := 0; attempt < maxRetryAttempts; attempt++ {
			if attempt > 0 {
				delay := calculateRetryDelay(attempt - 1)
				log.Warnf("Retrying profile upload to %s after %v (attempt %d/%d, payload size: %d bytes)", u.Host, delay, attempt+1, maxRetryAttempts, len(bodyBytes))
				time.Sleep(delay)
			}

			// Clone the request and set up a fresh body for this attempt
			attemptReq := req.Clone(req.Context())
			if bodyBytes != nil {
				attemptReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}
			setTarget(attemptReq, u, apiKey)

			// Use fresh transport for retries to avoid stale connection issues
			// that can cause 502 errors from load balancers
			roundTripper := m.rt
			if attempt > 0 {
				if transport, ok := m.rt.(*http.Transport); ok {
					// Clone the transport to get fresh connection pools
					// The idea is that we won't inherit any idle connections from the previous attempt
					freshTransport := transport.Clone()
					freshTransport.IdleConnTimeout = transport.IdleConnTimeout // retain existing timeout settings
					roundTripper = freshTransport
					log.Warnf("Using fresh transport for retry attempt %d to %s (payload size: %d bytes)", attempt+1, u.Host, len(bodyBytes))
				}
			}

			// Measure how long this attempt takes
			attemptStart := time.Now()
			resp, lastErr = roundTripper.RoundTrip(attemptReq)
			attemptDuration := time.Since(attemptStart)

			// Check if this is the final attempt
			isLastAttempt := attempt == maxRetryAttempts-1

			// Determine if we should retry
			shouldRetry := false
			if lastErr != nil {
				// Connection-level error - check if retryable
				shouldRetry = isRetryableError(lastErr) && !isLastAttempt
				if !shouldRetry && !isLastAttempt {
					log.Debugf("Non-retryable connection error for profile upload to %s (payload size: %d bytes, duration: %v): %v", u.Host, len(bodyBytes), attemptDuration, lastErr)
				}
			} else if resp != nil {
				// Got a response - check if status code indicates retryable condition
				shouldRetry = isRetryableStatusCode(resp.StatusCode) && !isLastAttempt
				if shouldRetry {
					log.Warnf("Retryable HTTP status %d for profile upload to %s (payload size: %d bytes, duration: %v)", resp.StatusCode, u.Host, len(bodyBytes), attemptDuration)
				}
			}

			if !shouldRetry {
				// Either success, non-retryable error, or final attempt - return result
				return resp, lastErr
			}

			// We're going to retry - close the response body to prevent leaks
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}

			if lastErr != nil {
				log.Warnf("Retryable connection error for profile upload to %s (payload size: %d bytes, duration: %v): %v", u.Host, len(bodyBytes), attemptDuration, lastErr)
			}
		}

		// This should never be reached due to the isLastAttempt logic above
		return resp, lastErr
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
		return retryRoundTrip(m.targets[0], m.keys[0])
	}

	// Multiple targets - body already read above
	for i, u := range m.targets {
		if i == 0 {
			// Main endpoint - return its response and error
			rresp, rerr = retryRoundTrip(u, m.keys[i])
			continue
		}

		// Additional endpoints - discard responses but still retry (synchronous)
		if resp, err := retryRoundTrip(u, m.keys[i]); err == nil {
			// Successfully sent to additional endpoint, discard response
			io.Copy(io.Discard, resp.Body) //nolint:errcheck
			resp.Body.Close()
		} else {
			log.Error(err)
		}
	}
	return rresp, rerr
}

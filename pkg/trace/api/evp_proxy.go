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
	"mime"
	"net/http"
	"net/http/httptrace"
	"net/http/httputil"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/trace/api/apiutil"
	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

const (
	validSubdomainSymbols       = "_-."
	validPathSymbols            = "/_-+"
	validPathQueryStringSymbols = "/_-+@?&=.:\"[]"
)

// EvpProxyAllowedHeaders contains the headers that the proxy will forward. All others will be cleared.
var EvpProxyAllowedHeaders = []string{"Content-Type", "Accept-Encoding", "Content-Encoding", "User-Agent", "DD-CI-PROVIDER-NAME", "DD-EVP-ORIGIN", "DD-EVP-ORIGIN-VERSION"}

// aiGuardProxyAllowedHeaders contains the headers that the AI Guard proxy will forward.
var aiGuardProxyAllowedHeaders = append(slices.Clone(EvpProxyAllowedHeaders), "DD-AI-GUARD-VERSION", "DD-AI-GUARD-SOURCE", "DD-AI-GUARD-LANGUAGE")

// evpProxyEndpointsFromConfig returns the configured list of endpoints to forward payloads to.
func evpProxyEndpointsFromConfig(conf *config.AgentConfig) []config.Endpoint {
	apiKey := conf.EVPProxy.APIKey
	if apiKey == "" {
		apiKey = conf.APIKey()
	}
	endpoint := conf.EVPProxy.DDURL
	if endpoint == "" {
		endpoint = conf.Site
	}
	endpoints := []config.Endpoint{{Host: endpoint, APIKey: apiKey}} // main endpoint
	for host, keys := range conf.EVPProxy.AdditionalEndpoints {
		for _, key := range keys {
			endpoints = append(endpoints, config.Endpoint{
				Host:   host,
				APIKey: key,
			})
		}
	}
	return endpoints
}

func (r *HTTPReceiver) evpProxyHandler(apiVersion int) http.Handler {
	if !r.conf.EVPProxy.Enabled {
		return evpProxyErrorHandler("Has been disabled in config")
	}
	handler := evpProxyForwarder(r.conf, r.statsd)
	return http.StripPrefix(fmt.Sprintf("/evp_proxy/v%d", apiVersion), handler)
}

// evpProxyErrorHandler returns an HTTP handler that will always return
// http.StatusMethodNotAllowed along with a clarification.
func evpProxyErrorHandler(message string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		msg := fmt.Sprintf("EVPProxy is disabled: %v", message)
		http.Error(w, msg, http.StatusMethodNotAllowed)
	})
}

// evpProxyForwarder creates an http.ReverseProxy which can forward payloads to
// one or more endpoints, based on the request received and the Agent configuration.
// Headers are not proxied, instead we add our own known set of headers.
// See also evpProxyTransport below.
func evpProxyForwarder(conf *config.AgentConfig, statsd statsd.ClientInterface) http.Handler {
	endpoints := evpProxyEndpointsFromConfig(conf)
	logger := stdlog.New(log.NewThrottled(5, 10*time.Second), "EVPProxy: ", 0) // limit to 5 messages every 10 seconds
	return &httputil.ReverseProxy{
		Rewrite: func(_ *httputil.ProxyRequest) {
			// The X-Forwarded-For header can be abused to fake the origin of requests and we don't need it,
			// so we do not call pr.SetXForwarded().
		},
		ErrorLog:  logger,
		Transport: &evpProxyTransport{conf.NewHTTPTransport(), endpoints, conf, NewContainerIDProviderFromConfig(conf), statsd, EvpProxyAllowedHeaders, false, false},
	}
}

// aiGuardProxyEnabled reports whether the Agent has both keys the AI Guard API requires.
func aiGuardProxyEnabled(conf *config.AgentConfig) bool {
	return conf.APIKey() != "" && conf.EVPProxy.ApplicationKey != ""
}

// aiGuardProxyHandler forwards AI Guard API requests to https://api.<site> with the
// Agent's API and application keys, through the main endpoint only.
func (r *HTTPReceiver) aiGuardProxyHandler() http.Handler {
	endpoints := []config.Endpoint{{Host: r.conf.Site, APIKey: r.conf.APIKey()}}
	// The handler reads the body under http.MaxBytesReader, so the transport must not limit it again.
	transport := &evpProxyTransport{r.conf.NewHTTPTransport(), endpoints, r.conf, NewContainerIDProviderFromConfig(r.conf), r.statsd, aiGuardProxyAllowedHeaders, true, true}
	throttled := log.NewThrottled(5, 10*time.Second)
	errorLog := stdlog.New(throttled, "AIGuardProxy: ", 0)
	return http.StripPrefix("/ai_guard/proxy", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// isValidPath rejects '.' in the decoded path, so dot segments cannot escape the AI Guard API.
		if (req.Method != http.MethodGet && req.Method != http.MethodPost) || req.URL.RawQuery != "" || req.URL.ForceQuery ||
			!strings.HasPrefix(req.URL.Path, "/api/v2/ai-guard/") || !isValidPath(req.URL.Path) {
			writeAIGuardProxyError(w, http.StatusNotFound, "path_not_allowed")
			return
		}
		if req.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "cross-site request rejected", http.StatusForbidden)
			return
		}
		if mediaType, _, _ := mime.ParseMediaType(req.Header.Get("Content-Type")); req.Method == http.MethodPost && mediaType != "application/json" {
			http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
			return
		}
		if limit := r.conf.EVPProxy.MaxPayloadSize; limit > 0 {
			req.Body = http.MaxBytesReader(w, req.Body, limit)
		}
		body, err := io.ReadAll(req.Body)
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeAIGuardProxyError(w, http.StatusRequestEntityTooLarge, "payload_too_large")
			return
		}
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		req.Body, req.ContentLength = io.NopCloser(bytes.NewReader(body)), int64(len(body))
		req.Header.Set("X-Datadog-EVP-Subdomain", "api")

		// Errors before a connection to Datadog is established (DNS, TCP or TLS) are told apart from later ones.
		var connected atomic.Bool
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
			GotConn: func(httptrace.GotConnInfo) { connected.Store(true) },
		}))
		proxy := &httputil.ReverseProxy{
			Rewrite:   func(_ *httputil.ProxyRequest) {},
			ErrorLog:  errorLog,
			Transport: transport,
			ModifyResponse: func(resp *http.Response) error {
				resp.Header.Del(aiGuardProxyErrorHeader)
				return nil
			},
			ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, err error) {
				code := "network_error"
				if !connected.Load() {
					code = "connection_failed"
				}
				throttled.Warn("AIGuardProxy: %s: %v", code, err)
				writeAIGuardProxyError(rw, http.StatusBadGateway, code)
			},
		}
		proxy.ServeHTTP(w, req)
	}))
}

// aiGuardProxyErrorHeader marks the responses of the AI Guard proxy written by the Agent itself,
// so that SDKs can tell them from Datadog's responses.
const aiGuardProxyErrorHeader = "Datadog-Agent-Proxy-Error"

// writeAIGuardProxyError writes an AI Guard proxy error with a JSON:API errors body.
func writeAIGuardProxyError(w http.ResponseWriter, status int, code string) {
	w.Header().Set(aiGuardProxyErrorHeader, code)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"errors":[{"status":"%d","code":%q,"title":%q}]}`, status, code, http.StatusText(status))
}

// evpProxyTransport sends HTTPS requests to multiple targets using an
// underlying http.RoundTripper. API keys are set separately for each target.
// When multiple endpoints are in use the response from the first endpoint
// is proxied back to the client, while for all aditional endpoints the
// response is discarded.
type evpProxyTransport struct {
	transport           http.RoundTripper
	endpoints           []config.Endpoint
	conf                *config.AgentConfig
	containerIDProvider IDProvider
	statsd              statsd.ClientInterface
	allowedHeaders      []string
	// sendApplicationKey forwards the Agent's application key as DD-APPLICATION-KEY. It is read
	// from the config per request, so rotating app_key takes effect without a restart.
	sendApplicationKey bool
	// bodyAlreadyLimited skips the limit below for a caller that enforced it itself. Applying it
	// twice rejects a body of exactly the limit: the limited reader reports the limit instead of
	// EOF on the read the HTTP transport makes after ContentLength bytes.
	bodyAlreadyLimited bool
}

func (t *evpProxyTransport) RoundTrip(req *http.Request) (rresp *http.Response, rerr error) {
	if req.Body != nil && !t.bodyAlreadyLimited && t.conf.EVPProxy.MaxPayloadSize > 0 {
		req.Body = apiutil.NewLimitedReader(req.Body, t.conf.EVPProxy.MaxPayloadSize)
	}
	start := time.Now()
	tags := []string{} // these tags are only for the debug metrics, not the payloads we forward
	if ct := req.Header.Get("Content-Type"); ct != "" {
		tags = append(tags, "content_type:"+ct)
	}
	defer func() {
		_ = t.statsd.Count("datadog.trace_agent.evp_proxy.request", 1, tags, 1)
		_ = t.statsd.Count("datadog.trace_agent.evp_proxy.request_bytes", req.ContentLength, tags, 1)
		_ = t.statsd.Timing("datadog.trace_agent.evp_proxy.request_duration_ms", time.Since(start), tags, 1)
		if rerr != nil {
			_ = t.statsd.Count("datadog.trace_agent.evp_proxy.request_error", 1, tags, 1)
		}
	}()

	subdomain := req.Header.Get("X-Datadog-EVP-Subdomain")
	containerID := t.containerIDProvider.GetContainerID(req.Context(), req.Header)

	// Sanitize the input, don't accept any valid URL but just some limited subset
	if len(subdomain) == 0 {
		return nil, errors.New("EVPProxy: no subdomain specified")
	}
	if !isValidSubdomain(subdomain) {
		return nil, fmt.Errorf("EVPProxy: invalid subdomain: %s", subdomain)
	}
	tags = append(tags, "subdomain:"+subdomain)
	if !isValidPath(req.URL.Path) {
		return nil, fmt.Errorf("EVPProxy: invalid target path: %s", req.URL.Path)
	}
	if !isValidQueryString(req.URL.RawQuery) {
		return nil, fmt.Errorf("EVPProxy: invalid query string: %s", req.URL.RawQuery)
	}

	// We don't want to forward arbitrary headers, create a copy of the input headers and clear them
	inputHeaders := req.Header
	req.Header = http.Header{}
	req.Trailer = nil // trailers are not part of the allowed headers, so they are not forwarded either

	// Set standard headers
	req.Header.Set("User-Agent", "") // Set to empty string so Go doesn't set its default
	req.Header.Set("Via", "trace-agent "+t.conf.AgentVersion)

	// Copy allowed headers from the input request
	for _, header := range t.allowedHeaders {
		val := inputHeaders.Get(header)
		if val != "" {
			req.Header.Set(header, val)
		}
	}

	// Set Datadog headers, except API key which is set per-endpoint
	if containerID != "" {
		req.Header.Set(header.ContainerID, containerID)
		if ctags := getContainerTags(t.conf.ContainerTags, containerID); ctags != "" {
			ctagsHeader := normalizeHTTPHeader(ctags)
			req.Header.Set("X-Datadog-Container-Tags", ctagsHeader)
			log.Debugf("Setting header X-Datadog-Container-Tags=%s for evp proxy", ctagsHeader)
		}
	}
	req.Header.Set("X-Datadog-Hostname", t.conf.Hostname)
	req.Header.Set("X-Datadog-AgentDefaultEnv", t.conf.DefaultEnv)
	log.Debugf("Setting headers X-Datadog-Hostnames=%s, X-Datadog-AgentDefaultEnv=%s for evp proxy", t.conf.Hostname, t.conf.DefaultEnv)
	req.Header.Set(header.ContainerID, containerID)
	if t.sendApplicationKey {
		if applicationKey := t.conf.EVPProxy.ApplicationKey; applicationKey != "" {
			req.Header.Set("DD-APPLICATION-KEY", applicationKey)
		}
	}
	if t.conf.ErrorTrackingStandalone {
		req.Header.Set("X-Datadog-Error-Tracking-Standalone", "true")
	}

	// Timeout: Our outbound request(s) can't take longer than the WriteTimeout of the server
	timeout := getConfiguredEVPRequestTimeoutDuration(t.conf)
	req.Header.Set("X-Datadog-Timeout", strconv.Itoa((int(timeout.Seconds()))))
	deadline := time.Now().Add(timeout)
	//nolint:govet // we don't need to manually cancel this context, we can rely on the parent context being cancelled
	ctx, _ := context.WithDeadline(req.Context(), deadline)
	req = req.WithContext(ctx)

	// Set target URL and API key header (per domain)
	req.URL.Scheme = "https"
	setTarget := func(r *http.Request, host, apiKey string) {
		targetHost := subdomain + "." + host
		r.Host = targetHost
		r.URL.Host = targetHost
		r.Header.Set("DD-API-KEY", apiKey)
	}

	// Shortcut if we only have one endpoint
	if len(t.endpoints) == 1 {
		setTarget(req, t.endpoints[0].Host, t.endpoints[0].APIKey)
		return t.transport.RoundTrip(req)
	}

	// There's more than one destination endpoint
	var slurp []byte
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		slurp = body
	}
	for i, endpointDomain := range t.endpoints {
		newreq := req.Clone(req.Context())
		if slurp != nil {
			newreq.Body = io.NopCloser(bytes.NewReader(slurp))
		}
		setTarget(newreq, endpointDomain.Host, endpointDomain.APIKey)
		if i == 0 {
			// given the way we construct the list of targets the main endpoint
			// will be the first one called, we return its response and error
			rresp, rerr = t.transport.RoundTrip(newreq)
			continue
		}

		if resp, err := t.transport.RoundTrip(newreq); err == nil {
			// we discard responses for all subsequent requests
			io.Copy(io.Discard, resp.Body) //nolint:errcheck
			resp.Body.Close()
		} else {
			log.Error(err)
		}
	}
	return rresp, rerr
}

// We don't want to accept any valid URL, we are strict in what we accept as a subdomain, path
// or query string to prevent abusing this API for other purposes than forwarding evp payloads.
func isValidSubdomain(s string) bool {
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && !strings.ContainsRune(validSubdomainSymbols, c) {
			return false
		}
	}
	return true
}

func isValidPath(s string) bool {
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && !strings.ContainsRune(validPathSymbols, c) {
			return false
		}
	}
	return true
}

func isValidQueryString(s string) bool {
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		log.Debugf("failed to unescape query string: %v", err)
		return false
	}
	for _, c := range decoded {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && !strings.ContainsRune(validPathQueryStringSymbols, c) {
			return false
		}
	}
	return true
}

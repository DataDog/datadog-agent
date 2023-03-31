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

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
)

const (
	// logsIntakeURLTemplate specifies the template for obtaining the intake URL along with the site.
	logsIntakeURLTemplate = "https://http-intake.logs.%s/api/v2/logs"
)

// debuggerProxyHandler returns an http.Handler proxying requests to the logs intake. If the logs intake url cannot be
// parsed, the returned handler will always return http.StatusInternalServerError with a clarifying message.
func (r *HTTPReceiver) debuggerProxyHandler() http.Handler {
	tags := fmt.Sprintf("host:%s,default_env:%s,agent_version:%s", r.conf.Hostname, r.conf.DefaultEnv, r.conf.AgentVersion)
	if orch := r.conf.FargateOrchestrator; orch != config.OrchestratorUnknown {
		tags = tags + ",orchestrator:fargate_" + strings.ToLower(string(orch))
	}
	intake := fmt.Sprintf(logsIntakeURLTemplate, r.conf.Site)
	if v := r.conf.DebuggerProxy.DDURL; v != "" {
		intake = v
	} else if site := r.conf.Site; site != "" {
		intake = fmt.Sprintf(logsIntakeURLTemplate, site)
	}
	target, err := url.Parse(intake)
	if err != nil {
		log.Criticalf("Error parsing debugger intake URL %q: %v", intake, err)
		return debuggerErrorHandler(fmt.Errorf("error parsing debugger intake URL %q: %v", intake, err))
	}
	apiKey := r.conf.APIKey()
	if k := r.conf.DebuggerProxy.APIKey; k != "" {
		apiKey = strings.TrimSpace(k)
	}
	targets := []*url.URL{target}
	apiKeys := []string{apiKey}
	additionalEndpoints := r.conf.DebuggerProxy.AdditionalEndpoints
	if additionalEndpoints != nil {
		for endpoint, keys := range additionalEndpoints {
			u, err := url.Parse(endpoint)
			if err != nil {
				log.Errorf("Error parsing additional debugger intake URL %s: %v", endpoint, err)
				continue
			}
			for _, key := range keys {
				targets = append(targets, u)
				apiKeys = append(apiKeys, strings.TrimSpace(key))
			}
		}
	}
	return newDebuggerProxy(r.conf, targets, apiKeys, tags)
}

// debuggerErrorHandler always returns http.StatusInternalServerError with a clarifying message.
func debuggerErrorHandler(err error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		msg := fmt.Sprintf("Debugger Proxy is OFF: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
	})
}

// newDebuggerProxy returns a new httputil.ReverseProxy proxying and augmenting requests with headers containing the tags.
func newDebuggerProxy(conf *config.AgentConfig, targets []*url.URL, keys []string, tags string) *httputil.ReverseProxy {
	logger := log.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	director := func(req *http.Request) {
		req.Header.Set("DD-REQUEST-ID", uuid.New().String())
		req.Header.Set("DD-EVP-ORIGIN", "agent-debugger")
	}
	return &httputil.ReverseProxy{
		Director: director,
		ErrorLog: stdlog.New(logger, "debugger.Proxy: ", 0),
		Transport: &measuringDebuggerMultiTransport{
			conf.NewHTTPTransport(),
			targets,
			keys,
			tags,
			conf,
			NewIDProvider(conf.ContainerProcRoot),
		},
	}
}

// measuringDebuggerMultiTransport sends HTTP requests to a defined target url. It also sets the API keys in the headers.
type measuringDebuggerMultiTransport struct {
	rt          http.RoundTripper
	targets     []*url.URL
	keys        []string
	tags        string
	conf        *config.AgentConfig
	cidProvider IDProvider
}

func (m *measuringDebuggerMultiTransport) RoundTrip(req *http.Request) (rres *http.Response, rerr error) {
	defer func(start time.Time) {
		var tags []string
		metrics.Count("datadog.trace_agent.debugger.proxy_request", 1, tags, 1)
		metrics.Timing("datadog.trace_agent.debugger.proxy_request_duration_ms", time.Since(start), tags, 1)
		if rerr != nil {
			tags := append(tags, fmt.Sprintf("error:%s", fmt.Sprintf("%T", rerr)))
			metrics.Count("datadog.trace_agent.debugger.proxy_request_error", 1, tags, 1)
		}
	}(time.Now())

	ddtags := m.tags
	containerID := m.cidProvider.GetContainerID(req.Context(), req.Header)
	if ct := getContainerTags(m.conf.ContainerTags, containerID); ct != "" {
		ddtags = fmt.Sprintf("%s,%s", ddtags, ct)
	}
	q := req.URL.Query()
	if qtags := q.Get("ddtags"); qtags != "" {
		ddtags = fmt.Sprintf("%s,%s", ddtags, qtags)
	}

	if len(m.targets) == 1 {
		m.setTarget(req, m.targets[0], m.keys[0], ddtags)
		return m.rt.RoundTrip(req)
	}

	slurp, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	for i, u := range m.targets {
		newreq := req.Clone(req.Context())
		newreq.Body = io.NopCloser(bytes.NewReader(slurp))
		m.setTarget(newreq, u, m.keys[i], ddtags)
		if i == 0 {
			// given the way we construct the list of targets the main endpoint
			// will be the first one called, we return its response and error
			rres, rerr = m.rt.RoundTrip(newreq)
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
	return rres, rerr
}

func (m *measuringDebuggerMultiTransport) setTarget(r *http.Request, u *url.URL, apiKey string, tags string) {
	q := r.URL.Query()
	q.Set("ddtags", tags)
	newTarget := u
	newTarget.RawQuery = q.Encode()
	r.Host = u.Host
	r.URL = newTarget
	r.Header.Set("DD-API-KEY", apiKey)
}

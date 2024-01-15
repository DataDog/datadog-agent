// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"fmt"
	stdlog "log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/google/uuid"
)

const (
	// logsIntakeURLTemplate is the template for building the logs intake URL for each site.
	logsIntakeURLTemplate = "https://http-intake.logs.%s/api/v2/logs"

	// debuggerDiagnosticsURLTemplate is the template for building the debugger intake URL for each site.
	debuggerDiagnosticsURLTemplate = "https://debugger-intake.%s/api/v2/debugger"

	// ddTagsQueryStringMaxLen is the maximum number of characters we send as ddtags in the intake query string.
	// This limit is not imposed by the event platform intake, it's a safeguard we've added to guarantee an upper
	// bound for the tags.
	ddTagsQueryStringMaxLen = 4001
)

// debuggerLogsProxyHandler returns an http.Handler proxying Dynamic Instrumentation dynamic logs
// to the logs intake.
func (r *HTTPReceiver) debuggerLogsProxyHandler() http.Handler {
	return r.debuggerProxyHandler(logsIntakeURLTemplate, r.conf.DebuggerProxy)
}

// debuggerDiagnosticsProxyHandler returns an http.Handler proxying Dynamic Instrumentation diagnostic messages
// to the debugger intake.
func (r *HTTPReceiver) debuggerDiagnosticsProxyHandler() http.Handler {
	return r.debuggerProxyHandler(debuggerDiagnosticsURLTemplate, r.conf.DebuggerDiagnosticsProxy)
}

// debuggerProxyHandler returns an http.Handler proxying requests to the configured intake. If the intake url cannot be
// parsed, the returned handler will always return http.StatusInternalServerError with a clarifying message.
func (r *HTTPReceiver) debuggerProxyHandler(urlTemplate string, proxyConfig config.DebuggerProxyConfig) http.Handler {
	hostTags := fmt.Sprintf("host:%s,default_env:%s,agent_version:%s", r.conf.Hostname, r.conf.DefaultEnv, r.conf.AgentVersion)
	if orch := r.conf.FargateOrchestrator; orch != config.OrchestratorUnknown {
		hostTags = hostTags + ",orchestrator:fargate_" + strings.ToLower(string(orch))
	}
	intake := fmt.Sprintf(urlTemplate, r.conf.Site)
	if v := proxyConfig.DDURL; v != "" {
		intake = v
	} else if site := r.conf.Site; site != "" {
		intake = fmt.Sprintf(urlTemplate, site)
	}
	target, err := url.Parse(intake)
	if err != nil {
		log.Criticalf("Error parsing debugger intake URL %q: %v", intake, err)
		return debuggerErrorHandler(fmt.Errorf("error parsing debugger intake URL %q: %v", intake, err))
	}
	apiKey := r.conf.APIKey()
	if k := proxyConfig.APIKey; k != "" {
		apiKey = strings.TrimSpace(k)
	}
	transport := newMeasuringForwardingTransport(
		r.conf.NewHTTPTransport(), target, apiKey, proxyConfig.AdditionalEndpoints, "datadog.trace_agent.debugger", []string{})
	return newDebuggerProxy(r.conf, transport, hostTags)
}

// debuggerErrorHandler always returns http.StatusInternalServerError with a clarifying message.
func debuggerErrorHandler(err error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		msg := fmt.Sprintf("Debugger Proxy is OFF: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
	})
}

// newDebuggerProxy returns a new httputil.ReverseProxy proxying and augmenting requests with headers containing the tags.
func newDebuggerProxy(conf *config.AgentConfig, transport http.RoundTripper, hostTags string) *httputil.ReverseProxy {
	cidProvider := NewIDProvider(conf.ContainerProcRoot)
	logger := log.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	return &httputil.ReverseProxy{
		Director:  getDirector(hostTags, cidProvider, conf.ContainerTags),
		ErrorLog:  stdlog.New(logger, "debugger.Proxy: ", 0),
		Transport: transport,
	}
}

func getDirector(hostTags string, cidProvider IDProvider, containerTags func(string) ([]string, error)) func(*http.Request) {
	return func(req *http.Request) {
		req.Header.Set("DD-REQUEST-ID", uuid.New().String())
		req.Header.Set("DD-EVP-ORIGIN", "agent-debugger")
		q := req.URL.Query()
		containerID := cidProvider.GetContainerID(req.Context(), req.Header)
		tags := hostTags
		if ctags := getContainerTags(containerTags, containerID); ctags != "" {
			tags = fmt.Sprintf("%s,%s", tags, ctags)
		}
		if htags := req.Header.Get("X-Datadog-Additional-Tags"); htags != "" {
			tags = fmt.Sprintf("%s,%s", tags, htags)
		}
		if qtags := q.Get("ddtags"); qtags != "" {
			tags = fmt.Sprintf("%s,%s", tags, qtags)
		}
		maxLen := len(tags)
		if maxLen > ddTagsQueryStringMaxLen {
			log.Warnf("Truncating tags in upload to %s. Got %d, max is %d.", req.URL.Path, maxLen, ddTagsQueryStringMaxLen)
			maxLen = ddTagsQueryStringMaxLen
		}
		tags = tags[0:maxLen]
		q.Set("ddtags", tags)
		req.URL.RawQuery = q.Encode()
	}
}

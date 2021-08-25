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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/logutil"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// logsIntakeURLTemplate specifies the template for obtaining the intake URL along with the site.
	logsIntakeURLTemplate = "https://http-intake.logs.%s/v1/input"
)

// debuggerProxyHandler returns an http.Handler proxying requests to the logs intake. If the logs intake url cannot be
// parsed, the returned handler will always return http.StatusInternalServerError with a clarifying message.
func (r *HTTPReceiver) debuggerProxyHandler() http.Handler {
	tags := fmt.Sprintf("host:%s,default_env:%s,agent_version:%s", r.conf.Hostname, r.conf.DefaultEnv, info.Version)
	if orch := r.conf.FargateOrchestrator; orch != fargate.Unknown {
		tags = tags + ",orchestrator:fargate_" + strings.ToLower(string(orch))
	}
	intake := fmt.Sprintf(logsIntakeURLTemplate, config.DefaultSite)
	if v := config.Datadog.GetString("apm_config.debugger_dd_url"); v != "" {
		intake = v
	} else if site := config.Datadog.GetString("site"); site != "" {
		intake = fmt.Sprintf(logsIntakeURLTemplate, site)
	}
	target, err := url.Parse(intake)
	if err != nil {
		log.Criticalf("Error parsing debugger intake URL %q: %v", intake, err)
		return debuggerErrorHandler(fmt.Errorf("error parsing debugger intake URL %q: %v", intake, err))
	}
	return newDebuggerProxy(r.conf.NewHTTPTransport(), target, r.conf.APIKey(), tags)
}

// debuggerErrorHandler always returns http.StatusInternalServerError with a clarifying message.
func debuggerErrorHandler(err error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		msg := fmt.Sprintf("Debugger Proxy is OFF: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
	})
}

// newDebuggerProxy returns a new httputil.ReverseProxy proxying and augmenting requests with headers containing the tags.
func newDebuggerProxy(rt http.RoundTripper, target *url.URL, key string, tags string) *httputil.ReverseProxy {
	logger := logutil.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	director := func(req *http.Request) {
		ddtags := tags
		containerID := req.Header.Get(headerContainerID)
		if ct := getContainerTags(containerID); ct != "" {
			ddtags = fmt.Sprintf("%s,%s", ddtags, ct)
		}
		q := req.URL.Query()
		if qtags := q.Get("ddtags"); qtags != "" {
			ddtags = fmt.Sprintf("%s,%s", ddtags, qtags)
		}
		q.Set("ddtags", ddtags)
		newTarget := *target
		newTarget.RawQuery = q.Encode()
		req.Header.Set("DD-API-KEY", key)
		req.URL = &newTarget
		req.Host = target.Host
	}
	return &httputil.ReverseProxy{
		Director:  director,
		ErrorLog:  stdlog.New(logger, "debugger.Proxy: ", 0),
		Transport: &measuringDebuggerTransport{rt},
	}
}

// measuringDebuggerTransport sends HTTP requests to a defined target url. It also sets the API keys in the headers.
type measuringDebuggerTransport struct {
	rt http.RoundTripper
}

func (m *measuringDebuggerTransport) RoundTrip(req *http.Request) (res *http.Response, err error) {
	defer func(start time.Time) {
		var tags []string
		metrics.Count("datadog.trace_agent.debugger.proxy_request", 1, tags, 1)
		metrics.Timing("datadog.trace_agent.debugger.proxy_request_duration_ms", time.Since(start), tags, 1)
		if err != nil {
			tags := append(tags, fmt.Sprintf("error:%s", fmt.Sprintf("%T", err)))
			metrics.Count("datadog.trace_agent.debugger.proxy_request_error", 1, tags, 1)
		}
	}(time.Now())
	return m.rt.RoundTrip(req)
}

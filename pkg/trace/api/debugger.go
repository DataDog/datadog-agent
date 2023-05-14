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
	// logsIntakeURLTemplate specifies the template for obtaining the intake URL along with the site.
	logsIntakeURLTemplate = "https://http-intake.logs.%s/api/v2/logs"

	// logsIntakeMaximumTagsLength is the maximum number of characters we send as ddtags.
	logsIntakeMaximumTagsLength = 4001
)

// debuggerProxyHandler returns an http.Handler proxying requests to the logs intake. If the logs intake url cannot be
// parsed, the returned handler will always return http.StatusInternalServerError with a clarifying message.
func (r *HTTPReceiver) debuggerProxyHandler() http.Handler {
	hostTags := fmt.Sprintf("host:%s,default_env:%s,agent_version:%s", r.conf.Hostname, r.conf.DefaultEnv, r.conf.AgentVersion)
	if orch := r.conf.FargateOrchestrator; orch != config.OrchestratorUnknown {
		hostTags = hostTags + ",orchestrator:fargate_" + strings.ToLower(string(orch))
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
	transport := newMeasuringForwardingTransport(
		r.conf.NewHTTPTransport(), target, apiKey, r.conf.DebuggerProxy.AdditionalEndpoints, "datadog.trace_agent.debugger", []string{})
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
		if maxLen > logsIntakeMaximumTagsLength {
			log.Warn("Truncating tags in debugger endpoint. Got %d, max is %d.", maxLen, logsIntakeMaximumTagsLength)
			maxLen = logsIntakeMaximumTagsLength
		}
		tags = tags[0:maxLen]
		q.Set("ddtags", tags)
		req.URL.RawQuery = q.Encode()
	}
}

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

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

const (
	// debuggerIntakeURLTemplate specifies the template for obtaining the intake URL along with the site.
	debuggerIntakeURLTemplate = "https://debugger-intake.%s/api/v2/debugger"
)

// symDBProxyHandler returns an http.Handler proxying requests to the logs intake. If the logs intake url cannot be
// parsed, the returned handler will always return http.StatusInternalServerError with a clarifying message.
func (r *HTTPReceiver) symDBProxyHandler() http.Handler {
	hostTags := fmt.Sprintf("host:%s,default_env:%s,agent_version:%s", r.conf.Hostname, r.conf.DefaultEnv, r.conf.AgentVersion)
	if orch := r.conf.FargateOrchestrator; orch != config.OrchestratorUnknown {
		hostTags = hostTags + ",orchestrator:fargate_" + strings.ToLower(string(orch))
	}
	intake := fmt.Sprintf(debuggerIntakeURLTemplate, r.conf.Site)
	if v := r.conf.SymDBProxy.DDURL; v != "" {
		intake = v
	} else if site := r.conf.Site; site != "" {
		intake = fmt.Sprintf(debuggerIntakeURLTemplate, site)
	}
	target, err := url.Parse(intake)
	if err != nil {
		log.Criticalf("Error parsing symbol database intake URL %q: %v", intake, err)
		return symDBErrorHandler(fmt.Errorf("error parsing symbol database intake URL %q: %v", intake, err))
	}
	apiKey := r.conf.APIKey()
	if k := r.conf.SymDBProxy.APIKey; k != "" {
		apiKey = strings.TrimSpace(k)
	}
	transport := newMeasuringForwardingTransport(
		config.New().NewHTTPTransport(), target, apiKey, r.conf.SymDBProxy.AdditionalEndpoints, "datadog.trace_agent.debugger.", []string{})
	return newSymDBProxy(r.conf, transport, hostTags)
}

// symDBErrorHandler always returns http.StatusInternalServerError with a clarifying message.
func symDBErrorHandler(err error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		msg := fmt.Sprintf("SymDB Proxy is OFF: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
	})
}

// newSymDBProxy returns a new httputil.ReverseProxy proxying and augmenting requests with headers containing the tags.
func newSymDBProxy(conf *config.AgentConfig, transport http.RoundTripper, hostTags string) *httputil.ReverseProxy {
	cidProvider := NewIDProvider(conf.ContainerProcRoot)
	logger := log.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	return &httputil.ReverseProxy{
		Director:  getSymDBDirector(hostTags, cidProvider, conf.ContainerTags),
		ErrorLog:  stdlog.New(logger, "symdb.Proxy: ", 0),
		Transport: transport,
	}
}

func getSymDBDirector(hostTags string, cidProvider IDProvider, containerTags func(string) ([]string, error)) func(*http.Request) {
	return func(req *http.Request) {
		req.Header.Set("DD-REQUEST-ID", uuid.New().String())
		req.Header.Set("DD-EVP-ORIGIN", "agent-symdb")
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
		req.Header.Set("X-Datadog-Additional-Tags", tags)
	}
}

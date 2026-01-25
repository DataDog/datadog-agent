// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

type transport int

const (
	undefined transport = iota
	uds
	tcp
)

func (t transport) String() string {
	switch t {
	case uds:
		return "uds"
	case tcp:
		return "tcp"
	case undefined:
		fallthrough
	default:
		return "undefined"
	}
}

type tracegenCfg struct {
	transport             transport
	addSpanTags           string
	enableClientSideStats bool
}

func runTracegenDocker(h *components.RemoteHost, service string, cfg tracegenCfg) (shutdown func()) {
	var run, rm string
	if cfg.transport == uds {
		run, rm = tracegenUDSCommands(service, cfg.addSpanTags, cfg.enableClientSideStats)
	} else if cfg.transport == tcp {
		run, rm = tracegenTCPCommands(service, cfg.addSpanTags, cfg.enableClientSideStats)
	}
	h.MustExecute(rm) // kill any existing leftover container
	h.MustExecute(run)
	return func() { h.MustExecute(rm) }
}

func tracegenUDSCommands(service string, peerTags string, enableClientSideStats bool) (string, string) {
	// TODO: use a proper docker-compose definition for tracegen
	// TRACEGEN_WAITTIME set to 5s, allowing the agent to resolve container tags before
	// payloads are produced. This fixes flakiness around container tags resolution.
	run := "docker run -d --rm --name " + service +
		" -v /var/run/datadog/:/var/run/datadog/ " +
		" -e DD_TRACE_AGENT_URL=unix:///var/run/datadog/apm.socket " +
		" -e DD_SERVICE=" + service +
		" -e DD_GIT_COMMIT_SHA=abcd1234 " +
		" -e TRACEGEN_ADDSPANTAGS=" + peerTags +
		" -e TRACEGEN_WAITTIME=5s " +
		" -e DD_TRACE_STATS_COMPUTATION_ENABLED=" + strconv.FormatBool(enableClientSideStats) +
		" ghcr.io/datadog/apps-tracegen:" + apps.Version
	rm := "docker rm -f " + service
	return run, rm
}

func tracegenTCPCommands(service string, peerTags string, enableClientSideStats bool) (string, string) {
	// TODO: use a proper docker-compose definition for tracegen
	// TRACEGEN_WAITTIME set to 5s, allowing the agent to resolve container tags before
	// payloads are produced. This fixes flakiness around container tags resolution.
	run := "docker run -d --network host --rm --name " + service +
		" -e DD_SERVICE=" + service +
		" -e DD_GIT_COMMIT_SHA=abcd1234 " +
		" -e TRACEGEN_ADDSPANTAGS=" + peerTags +
		" -e TRACEGEN_WAITTIME=5s " +
		" -e DD_TRACE_STATS_COMPUTATION_ENABLED=" + strconv.FormatBool(enableClientSideStats) +
		" ghcr.io/datadog/apps-tracegen:" + apps.Version
	rm := "docker rm -f " + service
	return run, rm
}

func traceWithProcessTagsWithHeader(h *components.RemoteHost, processTags, service string) {
	// TODO: once go tracer support process tags, use tracegen instead!
	h.MustExecute(fmt.Sprintf(`curl -X POST http://localhost:8126/v0.4/traces \
-H 'X-Datadog-Process-Tags: %s' \
-H 'X-Datadog-Trace-Count: 1' \
-H 'Content-Type: application/json' \
-H 'User-Agent: Go-http-client/1.1' \
-H 'Datadog-Meta-Lang: go' \
--data-binary @- <<EOF
[[{"trace_id":1234567890123456789,"span_id":9876543210987654321,"parent_id":0,"name":"http.request","resource":"GET /api/users","service":"%s","type":"web","start":0,"duration":200000000,"meta":{"http.method":"GET","http.url":"/api/users","http.status_code":"200","env":"dev","version":"1.0.0"},"metrics":{"_sampling_priority_v1":1}}]]
EOF`, processTags, service))
}

func traceWithProcessTags(h *components.RemoteHost, processTags, service string) {
	// TODO: once go tracer support process tags, use tracegen instead!
	h.MustExecute(fmt.Sprintf(`curl -X POST http://localhost:8126/v0.4/traces \
-H 'X-Datadog-Trace-Count: 1' \
-H 'Content-Type: application/json' \
-H 'User-Agent: Go-http-client/1.1' \
-H 'Datadog-Meta-Lang: go' \
--data-binary @- <<EOF
[[{"trace_id":1234567890123456789,"span_id":9876543210987654321,"parent_id":0,"name":"http.request","resource":"GET /api/users","service":"%s","type":"web","start":0,"duration":200000000,"meta":{"_dd.tags.process":"%s","http.method":"GET","http.url":"/api/users","http.status_code":"200","env":"dev","version":"1.0.0"},"metrics":{"_sampling_priority_v1":1}}]]
EOF`, service, processTags))
}

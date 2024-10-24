// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
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
	transport   transport
	addSpanTags string
}

func runTracegenDocker(h *components.RemoteHost, service string, cfg tracegenCfg) (shutdown func()) {
	var run, rm string
	if cfg.transport == uds {
		run, rm = tracegenUDSCommands(service, cfg.addSpanTags)
	} else if cfg.transport == tcp {
		run, rm = tracegenTCPCommands(service, cfg.addSpanTags)
	}
	h.MustExecute(rm) // kill any existing leftover container
	h.MustExecute(run)
	return func() { h.MustExecute(rm) }
}

func tracegenUDSCommands(service string, peerTags string) (string, string) {
	// TODO: use a proper docker-compose definition for tracegen
	run := "docker run -d --rm --name " + service +
		" -v /var/run/datadog/:/var/run/datadog/ " +
		" -e DD_TRACE_AGENT_URL=unix:///var/run/datadog/apm.socket " +
		" -e DD_SERVICE=" + service +
		" -e DD_GIT_COMMIT_SHA=abcd1234 " +
		" -e TRACEGEN_ADDSPANTAGS=" + peerTags +
		" ghcr.io/datadog/apps-tracegen:main"
	rm := "docker rm -f " + service
	return run, rm
}

func tracegenTCPCommands(service string, peerTags string) (string, string) {
	// TODO: use a proper docker-compose definition for tracegen
	run := "docker run -d --network host --rm --name " + service +
		" -e DD_SERVICE=" + service +
		" -e DD_GIT_COMMIT_SHA=abcd1234 " +
		" -e TRACEGEN_ADDSPANTAGS=" + peerTags +
		" ghcr.io/datadog/apps-tracegen:main"
	rm := "docker rm -f " + service
	return run, rm
}

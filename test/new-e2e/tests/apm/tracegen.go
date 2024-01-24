// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import "github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"

type tracegenCfg struct {
	transport transport
}

func runTracegen(h *components.RemoteHost, service string, cfg tracegenCfg) (shutdown func()) {
	var run, rm string
	if cfg.transport == UDS {
		run, rm = tracegenUDSCommands(service)
	} else if cfg.transport == TCP {
		run, rm = tracegenTCPCommands(service)
	}
	h.MustExecute(rm) // kill any existing leftover container
	h.MustExecute(run)
	return func() { h.MustExecute(rm) }
}

func tracegenUDSCommands(service string) (string, string) {
	// TODO: use a proper docker-compose definition for tracegen
	run := "docker run -d --rm --name " + service +
		" -v /var/run/datadog/:/var/run/datadog/ " +
		" -e DD_TRACE_AGENT_URL=unix:///var/run/datadog/apm.socket " +
		" -e DD_SERVICE=" + service +
		" ghcr.io/datadog/apps-tracegen:main"
	rm := "docker rm -f " + service
	return run, rm
}

func tracegenTCPCommands(service string) (string, string) {
	// TODO: use a proper docker-compose definition for tracegen
	run := "docker run -d --network host --rm --name " + service +
		" -e DD_SERVICE=" + service +
		" ghcr.io/datadog/apps-tracegen:main"
	rm := "docker rm -f " + service
	return run, rm
}

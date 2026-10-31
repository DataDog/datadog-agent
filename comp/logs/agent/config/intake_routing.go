// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"slices"

	pkgconfigutils "github.com/DataDog/datadog-agent/pkg/config/utils"
)

// HostOverride is the host and port intake routing sends an endpoint's logs to instead of its
// configured ones.
type HostOverride struct {
	Host string
	// Port 0 means the default HTTPS port.
	Port int
}

// IsLogsIntakeRoutingCandidate reports whether intake routing may send this endpoint's logs to
// another host. Only an HTTPS endpoint of the official core logs intake qualifies:
// agent-http-intake.logs.<site>, whether it comes from 'site' or is set explicitly, on the default
// port, with no path prefix, using the v2 API. MRF, Observability Pipelines Worker, dedicated
// PrivateLink hostnames, proxied and custom endpoints do not. PrivateLink with private DNS keeps
// the official hostname: the intake routing component detects it by resolving the host. The caller
// must also check that the endpoints use HTTP.
func (e *Endpoint) IsLogsIntakeRoutingCandidate() bool {
	return !e.IsMRF &&
		!e.isObsPipelineWorker &&
		e.useSSL &&
		e.PathPrefix == EmptyPathPrefix &&
		(e.Port == 0 || e.Port == 443) &&
		e.Version == EPIntakeVersion2 &&
		pkgconfigutils.IsDatadogIntakeHost(httpEndpointPrefix, e.Host)
}

// LogsIntakeRoutingCandidates returns the indices, in Endpoints, of the endpoints intake routing
// may send to another host. It returns nil when the endpoints do not use HTTP.
func (e *Endpoints) LogsIntakeRoutingCandidates() []int {
	if !e.UseHTTP {
		return nil
	}
	var indices []int
	for i := range e.Endpoints {
		if e.Endpoints[i].IsLogsIntakeRoutingCandidate() {
			indices = append(indices, i)
		}
	}
	return indices
}

// WithHostOverrides returns a copy of e where the endpoints at the given indices of Endpoints
// send to another host and port. Main is overridden along with Endpoints[0]. The receiver is
// never modified, and it is returned as is when there is no override. The copies share their
// credentials with the receiver, so API key rotation keeps reaching them.
func (e *Endpoints) WithHostOverrides(overrides map[int]HostOverride) *Endpoints {
	if len(overrides) == 0 {
		return e
	}
	routed := *e
	routed.Endpoints = slices.Clone(e.Endpoints)
	for idx, override := range overrides {
		if idx < 0 || idx >= len(routed.Endpoints) {
			continue
		}
		routed.Endpoints[idx].applyHostOverride(override)
		if idx == 0 {
			routed.Main.applyHostOverride(override)
		}
	}
	return &routed
}

func (e *Endpoint) applyHostOverride(override HostOverride) {
	if e.routedFrom == "" {
		e.routedFrom = e.Host
	}
	e.Host = override.Host
	e.Port = override.Port
}

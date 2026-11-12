// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"fmt"
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

// LogsIntakeRoutingExclusion returns why intake routing may not send this endpoint's logs to
// another host, or "" when it may. Only an HTTPS endpoint of the official logs intake qualifies: a
// host rooted under the logs subdomain of a Datadog site (<labels>.logs.<site>, such as
// agent-http-intake.logs.<site>), whether it comes from 'site' or is set explicitly, on the default
// port, with no path prefix, using the v2 API. MRF, Observability Pipelines Worker, proxied and
// custom endpoints do not. PrivateLink keeps the official hostnames, which private DNS resolves to
// the VPC endpoint, so it qualifies here: the intake routing component detects it by resolving the
// host, and keeps such endpoints on their configured host. Only endpoints sent over HTTP can be
// routed: the caller must also check Endpoints.UseHTTP.
func (e *Endpoint) LogsIntakeRoutingExclusion() string {
	switch {
	case e.IsMRF:
		return "it is a Multi-Region Failover endpoint"
	case e.isObsPipelineWorker:
		return "it is an Observability Pipelines Worker endpoint"
	case !e.useSSL:
		return "it does not use SSL"
	case e.PathPrefix != EmptyPathPrefix:
		return fmt.Sprintf("it has the path prefix %q", e.PathPrefix)
	case e.Port != 0 && e.Port != 443:
		return fmt.Sprintf("it uses port %d instead of 443", e.Port)
	case e.Version != EPIntakeVersion2:
		return "it does not use the v2 intake API"
	case !pkgconfigutils.IsDatadogLogsHost(e.Host):
		return "its host is not an official Datadog logs intake host (<name>.logs.<site>)"
	default:
		return ""
	}
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

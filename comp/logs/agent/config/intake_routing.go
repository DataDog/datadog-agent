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

// Reasons IsIntakeRoutingEligible gives for an endpoint that intake routing may not send to another
// host.
const (
	IntakeRoutingIneligibleMRF            = "Multi-Region Failover endpoints are unsupported"
	IntakeRoutingIneligibleObsPipeline    = "Observability Pipelines Worker endpoints are unsupported"
	IntakeRoutingIneligibleNoSSL          = "endpoint must use SSL"
	IntakeRoutingIneligiblePathPrefix     = "endpoint must not specify a path prefix"
	IntakeRoutingIneligiblePort           = "endpoint must use port 443"
	IntakeRoutingIneligibleIntakeVersion  = "endpoint does not support the v2 intake API"
	IntakeRoutingIneligibleUnofficialHost = "detected non-official Datadog logs intake host"
)

// IsIntakeRoutingEligible reports whether intake routing may send this endpoint's logs to another
// host. When it may not, it also returns the reason, one of the IntakeRoutingIneligible constants;
// the reason is only meaningful when the endpoint is not eligible. Only endpoints sent over HTTP can
// be routed: the caller must also check Endpoints.UseHTTP.
func (e *Endpoint) IsIntakeRoutingEligible() (bool, string) {
	// Multi-Region Failover endpoints send to the failover region on purpose.
	if e.IsMRF {
		return false, IntakeRoutingIneligibleMRF
	}
	// Observability Pipelines Worker endpoints send to the customer's own workers, even when their
	// URL looks like an official intake.
	if e.isObsPipelineWorker {
		return false, IntakeRoutingIneligibleObsPipeline
	}
	// The Agent always sends to an override over HTTPS, so the configured endpoint must use it too.
	if !e.useSSL {
		return false, IntakeRoutingIneligibleNoSSL
	}
	// A path prefix means a proxy or a custom endpoint, not the intake itself.
	if e.PathPrefix != EmptyPathPrefix {
		return false, IntakeRoutingIneligiblePathPrefix
	}
	// The official intake listens on the default HTTPS port. Another port means a proxy or a
	// custom endpoint.
	if e.Port != 0 && e.Port != 443 {
		return false, IntakeRoutingIneligiblePort
	}
	// Overrides only accept the v2 intake API.
	if e.Version != EPIntakeVersion2 {
		return false, IntakeRoutingIneligibleIntakeVersion
	}
	// The host must be under the logs subdomain of a Datadog site (<name>.logs.<site>, such as
	// agent-http-intake.logs.<site>), whether it comes from 'site' or is set explicitly. Proxies
	// and custom hosts are not. PrivateLink keeps the official hostnames, which private DNS
	// resolves to the VPC endpoint, so it is eligible here: the intake routing component detects
	// it by resolving the host, and keeps such endpoints on their configured host.
	if !pkgconfigutils.IsDatadogLogsHost(e.Host) {
		return false, IntakeRoutingIneligibleUnofficialHost
	}
	return true, ""
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

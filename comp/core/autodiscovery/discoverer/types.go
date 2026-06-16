// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package discoverer owns configuration-discovery for Autodiscovery templates
// that carry a non-nil Discovery field. Such templates do not resolve through
// the regular template variable substitution path; instead, the integration
// itself is asked to produce the runtime instance config given live service info.
package discoverer

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// PermFail wraps an error to signal the worker that retrying will never
// succeed. The job is dropped immediately without consuming any retry budget.
type PermFail struct{ Err error }

func (e PermFail) Error() string { return e.Err.Error() }
func (e PermFail) Unwrap() error { return e.Err }

// Boundary between Autodiscovery and the discovery_config implementation.
// Agent serializes service info as JSON to the integration. Returns JSON configs.
type ConfigDiscoverer interface {
	DiscoverConfig(integrationName, serviceJSON string) (string, error)
}

// ServiceInfo is the subset of listeners.Service that the discoverer needs.
type ServiceInfo interface {
	GetServiceID() string
	GetHosts() (map[string]string, error)
	GetPorts() ([]workloadmeta.ContainerPort, error)
}

// ServiceLookup hands the worker a live ServiceInfo for a given service ID.
type ServiceLookup interface {
	LookupService(svcID string) (ServiceInfo, bool)
}

// ResultCallback receives the discovered configs after a successful probe.
// svcID and tplDigest identify the template-and-service pair the result is for.
type ResultCallback func(svcID, tplDigest string, configs []integration.Config)

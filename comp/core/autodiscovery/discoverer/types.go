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
	discovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/discoverer/def"
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

// ServiceInfo is the service boundary shared with the config manager.
type ServiceInfo = discovery.ServiceInfo

// ServiceLookup retrieves the current service before a probe runs.
type ServiceLookup = discovery.ServiceLookup

// ResultCallback delivers probe results to the config manager.
type ResultCallback = discovery.ResultCallback

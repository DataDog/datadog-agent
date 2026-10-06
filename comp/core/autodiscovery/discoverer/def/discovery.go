// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package discovery defines the optional configuration-discovery capability.
// It must not import the discovery engine or any probe implementation.
package discovery

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// ServiceInfo is the subset of a service needed by a discovery probe.
type ServiceInfo interface {
	GetServiceID() string
	GetHosts() (map[string]string, error)
	GetPorts() ([]workloadmeta.ContainerPort, error)
}

// ServiceLookup retrieves the current service before a probe runs.
type ServiceLookup interface {
	LookupService(svcID string) (ServiceInfo, bool)
}

// ResultCallback delivers probe results for a template-and-service pair.
// The config manager revalidates that pair before applying the results.
type ResultCallback func(svcID, tplDigest string, configs []integration.Config)

// Factory creates a config-manager-local discovery engine. Products without
// configuration discovery omit this capability entirely. NewWorker must return
// a non-nil worker and must not deliver results until Start is called.
type Factory interface {
	NewWorker(ServiceLookup, ResultCallback, *telemetry.Store) Worker
}

// Worker owns optional probing and discovery-specific config transformation.
// ResolveConfig is called with the config-manager lock held; it must not call
// back into ServiceLookup or ResultCallback. Reconciliation, secret resolution,
// and scheduling remain the config manager's responsibility.
type Worker interface {
	Enqueue(svcID, tplDigest, integrationName string)
	Start()
	Stop()
	ResolveConfig(template, discovered integration.Config, service listeners.Service) (integration.Config, error)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package gpupodresources reports PodResources API availability problems that
// prevent Kubernetes GPU workload attribution.
package gpupodresources

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	// IssueName is the human-readable issue name for PodResources API failures.
	IssueName = "GPU PodResources API Unavailable"
	// IssueType is IssueName lowercased with spaces replaced by underscores.
	IssueType = "gpu_podresources_api_unavailable"
	// IssueID is the stable instance ID prefix for PodResources API failures.
	IssueID = "gpu-podresources-api-unavailable"
)

const checkSource = "gpu-podresources"

// Requires defines the dependencies for the issue module.
type Requires struct {
	compdef.In
	Config    config.Component
	Hostname  hostnameinterface.Component
	SelfIdent *selfident.SelfIdent
}

// Provides defines the issue modules contributed to the registry.
type Provides struct {
	compdef.Out
	Module issues.Module `group:"healthplatform_issue"`
}

// Module provides the issue modules to the health platform registry.
func Module() fxutil.Module {
	return fxutil.Component(fxutil.ProvideComponentConstructor(newModule))
}

type gpuPodResourcesModule struct {
	cfg     config.Component
	checker *checker
}

func newModule(reqs Requires) Provides {
	return Provides{Module: &gpuPodResourcesModule{cfg: reqs.Config, checker: newChecker(reqs.Config, reqs.Hostname, reqs.SelfIdent)}}
}

func (m *gpuPodResourcesModule) IssueName() string {
	return IssueName
}

func (m *gpuPodResourcesModule) IssueType() string {
	return IssueType
}

func (m *gpuPodResourcesModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return GPUPodResourcesIssue{}.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck probes the API while GPU monitoring is enabled.
// The Kubernetes gate belongs inside Fn so disabling GPU monitoring resolves an
// issue left by a previous run.
func (m *gpuPodResourcesModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return &runnerdef.BuiltInPeriodicHealthCheck{
		BuiltInHealthCheck: runnerdef.BuiltInHealthCheck{
			Source: checkSource,
			Fn: func() ([]runnerdef.IssueReport, error) {
				if !m.cfg.GetBool("gpu.enabled") || !env.IsKubernetes() {
					return nil, nil
				}
				return m.checker.Run()
			},
		},
	}
}

// BuiltInStartupHealthCheck returns nil because the PodResources socket can
// become available after the Agent starts.
func (m *gpuPodResourcesModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package openshift

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
)

// Params holds the cluster and agent options shared across all OpenShift scenario variants.
type Params struct {
	kubeComp.OpenShiftClusterArgs
	AgentOptions []kubernetesagentparams.Option
}

// WithPullSecretPath sets the path to the OpenShift pull secret file.
func WithPullSecretPath(path string) func(*Params) error {
	return func(p *Params) error {
		p.PullSecretPath = path
		return nil
	}
}

// WithCPUs sets the number of CPUs for the OpenShift cluster.
func WithCPUs(cpus string) func(*Params) error {
	return func(p *Params) error {
		p.CPUs = cpus
		return nil
	}
}

// WithMemory sets the memory for the OpenShift cluster.
func WithMemory(memory string) func(*Params) error {
	return func(p *Params) error {
		p.Memory = memory
		return nil
	}
}

// WithDisk sets the disk size for the OpenShift cluster.
func WithDisk(disk string) func(*Params) error {
	return func(p *Params) error {
		p.Disk = disk
		return nil
	}
}

// WithAgentOptions returns an option that appends agent options.
func WithAgentOptions(opts ...kubernetesagentparams.Option) func(*Params) error {
	return func(p *Params) error {
		p.AgentOptions = append(p.AgentOptions, opts...)
		return nil
	}
}

// WithoutAgent returns an option that disables agent installation.
func WithoutAgent() func(*Params) error {
	return func(p *Params) error {
		p.AgentOptions = nil
		return nil
	}
}

// ApplyAgentEnvironment populates agent options from the common environment config.
// Callers should apply any provider-specific options (e.g. dual shipping) after this call.
func ApplyAgentEnvironment(p *Params, e config.Env) {
	if !e.AgentDeploy() {
		p.AgentOptions = nil
	}
}

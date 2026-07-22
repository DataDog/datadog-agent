// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package local

import (
	config "github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	localNamerNamespace = "local"
	// local Infra (local)
	DDInfraDefaultPublicKeyPath    = "local/defaultPublicKeyPath"
	DDInfraOpenShiftPullSecretPath = "local/openshift/pullSecretPath"
	DDInfraOpenShiftCPUs           = "local/openshift/cpus"
	DDInfraOpenShiftMemory         = "local/openshift/memory"
	DDInfraOpenShiftDisk           = "local/openshift/disk"
	DDInfraVMCPUs                  = "local/vm/cpus"
	DDInfraVMMemory                = "local/vm/memory"
	DDInfraVMDisk                  = "local/vm/disk"
	DDInfraVMHostname              = "local/vm/hostname"
)

type Environment struct {
	*config.CommonEnvironment

	Namer namer.Namer
}

var _ config.Env = (*Environment)(nil)

func NewEnvironment(ctx *pulumi.Context) (Environment, error) {
	env := Environment{
		Namer: namer.NewNamer(ctx, localNamerNamespace),
	}

	commonEnv, err := config.NewCommonEnvironment(ctx)
	if err != nil {
		return Environment{}, err
	}

	env.CommonEnvironment = &commonEnv

	return env, nil
}

// Cross Cloud Provider config

// InternalRegistry returns the internal registry.
// Local runs still pull agent-qa/cluster-agent-qa images from the AWS Agent QA
// ECR registry; authentication is handled by the ImagePullRegistry-based
// imagePullSecret mechanism (E2E_IMAGE_PULL_REGISTRY/USERNAME/PASSWORD), not by
// this environment.
func (e *Environment) InternalRegistry() string {
	return "669783387624.dkr.ecr.us-east-1.amazonaws.com"
}

// InternalDockerhubMirror returns the internal Dockerhub mirror.
func (e *Environment) InternalDockerhubMirror() string {
	return "registry-1.docker.io"
}

// InternalRegistryImageTagExists returns true if the image tag exists in the internal registry.
func (e *Environment) InternalRegistryImageTagExists(_, _ string) (bool, error) {
	return true, nil
}

// InternalRegistryFullImagePathExists returns true if the image and tag exists in the internal registry.
func (e *Environment) InternalRegistryFullImagePathExists(_ string) (bool, error) {
	return true, nil
}

// Common
func (e *Environment) DefaultPublicKeyPath() string {
	return e.InfraConfig.Get(DDInfraDefaultPublicKeyPath)
}

// OpenShiftPullSecretPath returns the path to the OpenShift pull secret file
func (e *Environment) OpenShiftPullSecretPath() string {
	return e.InfraConfig.Get(DDInfraOpenShiftPullSecretPath)
}

// VMCPUs returns the number of CPUs to allocate to a local VM (default: 2).
func (e *Environment) VMCPUs() string {
	if v := e.InfraConfig.Get(DDInfraVMCPUs); v != "" {
		return v
	}
	return "2"
}

// VMMemory returns the memory to allocate to a local VM (default: 4G).
func (e *Environment) VMMemory() string {
	if v := e.InfraConfig.Get(DDInfraVMMemory); v != "" {
		return v
	}
	return "4G"
}

// VMDisk returns the disk size to allocate to a local VM (default: 10G).
func (e *Environment) VMDisk() string {
	if v := e.InfraConfig.Get(DDInfraVMDisk); v != "" {
		return v
	}
	return "10G"
}

// VMHostname returns the hostname to configure on the agent for a local VM.
// Defaults to the Pulumi stack name so each deployment has a unique, identifiable host in Datadog.
func (e *Environment) VMHostname() string {
	if v := e.InfraConfig.Get(DDInfraVMHostname); v != "" {
		return v
	}
	return e.Ctx().Stack()
}

// OpenShiftCPUs returns the number of CPUs to allocate to the CRC cluster (default: 8).
func (e *Environment) OpenShiftCPUs() string {
	if v := e.InfraConfig.Get(DDInfraOpenShiftCPUs); v != "" {
		return v
	}
	return "8"
}

// OpenShiftMemory returns the memory in MB to allocate to the CRC cluster (default: 16384).
func (e *Environment) OpenShiftMemory() string {
	if v := e.InfraConfig.Get(DDInfraOpenShiftMemory); v != "" {
		return v
	}
	return "16384"
}

// OpenShiftDisk returns the disk size in GB to allocate to the CRC cluster (default: 50).
func (e *Environment) OpenShiftDisk() string {
	if v := e.InfraConfig.Get(DDInfraOpenShiftDisk); v != "" {
		return v
	}
	return "50"
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// Aliases to conf package
type (
	// Proxy alias to model.Proxy
	Proxy = model.Proxy
	// Reader is alias to model.Reader
	Reader = model.Reader
	// Writer is alias to model.Reader
	Writer = model.Writer
	// ReaderWriter is alias to model.ReaderWriter
	ReaderWriter = model.ReaderWriter
	// Loader is alias to model.Loader
	Loader = model.Loader
	// Config is alias to model.Config
	Config = model.Config
)

// NewConfig is alias for Config object.
var NewConfig = model.NewConfig

// Warnings represent the warnings in the config
type Warnings = model.Warnings

// environment Aliases
var (
	IsFeaturePresent             = env.IsFeaturePresent
	IsECS                        = env.IsECS
	IsKubernetes                 = env.IsKubernetes
	IsECSFargate                 = env.IsECSFargate
	IsServerless                 = env.IsServerless
	IsContainerized              = env.IsContainerized
	IsDockerRuntime              = env.IsDockerRuntime
	GetEnvDefault                = env.GetEnvDefault
	IsHostProcAvailable          = env.IsHostProcAvailable
	IsHostSysAvailable           = env.IsHostSysAvailable
	IsAnyContainerFeaturePresent = env.IsAnyContainerFeaturePresent
	GetDetectedFeatures          = env.GetDetectedFeatures
)

type (
	// Feature Alias
	Feature = env.Feature
	// FeatureMap Alias
	FeatureMap = env.FeatureMap
)

// Aliases for constants
const (
	ECSFargate               = env.ECSFargate
	Podman                   = env.Podman
	Docker                   = env.Docker
	EKSFargate               = env.EKSFargate
	ECSEC2                   = env.ECSEC2
	Kubernetes               = env.Kubernetes
	CloudFoundry             = env.CloudFoundry
	Cri                      = env.Cri
	Containerd               = env.Containerd
	KubeOrchestratorExplorer = env.KubeOrchestratorExplorer
)

// IsAutoconfigEnabled is alias for model.IsAutoconfigEnabled
func IsAutoconfigEnabled() bool {
	return env.IsAutoconfigEnabled(Datadog)
}

// Aliases for config overrides
var (
	AddOverride        = model.AddOverride
	AddOverrides       = model.AddOverrides
	AddOverrideFunc    = model.AddOverrideFunc
	applyOverrideFuncs = model.ApplyOverrideFuncs
)

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/pkg/conf"
	"github.com/DataDog/datadog-agent/pkg/conf/env"
)

// Aliases to conf package
type (
	// Proxy alias to conf.Proxy
	Proxy = conf.Proxy
	// Reader is alias to Conf.Reader
	Reader = conf.Reader
	// Writer is alias to Conf.Reader
	Writer = conf.Writer
	// ReaderWriter is alias to Conf.ReaderWriter
	ReaderWriter = conf.ReaderWriter
	// Loader is alias to Conf.Loader
	Loader = conf.Loader
	// Config is alias to conf.Config
	Config = conf.Config
)

// NewConfig is alias for Config object.
var NewConfig = conf.NewConfig

// Warnings represent the warnings in the config
type Warnings = conf.Warnings

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

// IsAutoconfigEnabled is alias for conf.IsAutoconfigEnabled
func IsAutoconfigEnabled() bool {
	return env.IsAutoconfigEnabled(Datadog)
}

// Aliases for config overrides
var (
	AddOverride        = conf.AddOverride
	AddOverrides       = conf.AddOverrides
	AddOverrideFunc    = conf.AddOverrideFunc
	applyOverrideFuncs = conf.ApplyOverrideFuncs
)

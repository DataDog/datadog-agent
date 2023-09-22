// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/pkg/conf"
	"github.com/DataDog/datadog-agent/pkg/conf/env"
	"github.com/DataDog/datadog-agent/pkg/config/configsetup"
)

// Proxy represents the configuration for proxies in the agent
type Proxy = conf.Proxy

// ConfigReader is a subset of Config that only allows reading of configuration
type ConfigReader = conf.ConfigReader

type ConfigWriter = conf.ConfigWriter

type ConfigReaderWriter = conf.ConfigReaderWriter

type ConfigLoader = conf.ConfigLoader

// Config represents an object that can load and store configuration parameters
// coming from different kind of sources:
// - defaults
// - files
// - environment variables
// - flags
type Config = conf.Config

var NewConfig = conf.NewConfig

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
	IsAutoconfigEnabled          = env.IsAutoconfigEnabled
	GetDetectedFeatures          = env.GetDetectedFeatures
)

type (
	Feature    = env.Feature
	FeatureMap = env.FeatureMap
)

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

var (
	StandardStatsdPrefixes  = configsetup.StandardStatsdPrefixes
	StandardJMXIntegrations = configsetup.StandardJMXIntegrations
)

func GetProcessAPIAddressPort() (string, error) {
	return configsetup.GetProcessAPIAddressPort(Datadog)
}

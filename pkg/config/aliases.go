// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	slog "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/logs"
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

// LoggerName Alias
type LoggerName = logs.LoggerName

// Aliases for  logs
var (
	NewLogWriter   = logs.NewLogWriter
	ChangeLogLevel = logs.ChangeLogLevel
)

// SetupLogger Alias using Datadog config
func SetupLogger(loggerName LoggerName, logLevel, logFile, syslogURI string, syslogRFC, logToConsole, jsonFormat bool) error {
	return logs.SetupLogger(loggerName, logLevel, logFile, syslogURI, syslogRFC, logToConsole, jsonFormat, Datadog)
}

// SetupJMXLogger Alias using Datadog config
func SetupJMXLogger(logFile, syslogURI string, syslogRFC, logToConsole, jsonFormat bool) error {
	return logs.SetupJMXLogger(logFile, syslogURI, syslogRFC, logToConsole, jsonFormat, Datadog)
}

// GetSyslogURI Alias using Datadog config
func GetSyslogURI() string {
	return logs.GetSyslogURI(Datadog)
}

// SetupDogstatsdLogger Alias using Datadog config
func SetupDogstatsdLogger(logFile string) (slog.LoggerInterface, error) {
	return logs.SetupDogstatsdLogger(logFile, Datadog)
}

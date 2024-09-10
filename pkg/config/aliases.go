// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config defines the configuration of the agent
package config

import (
	"context"

	slog "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
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
	// Config is alias to model.Config
	Config = model.Config
)

// NewConfig is alias for Config object.
var NewConfig = model.NewConfig

// Warnings represent the warnings in the config
type Warnings = model.Warnings

var (
	// Datadog Alias
	Datadog = pkgconfigsetup.Datadog
	// SystemProbe Alias
	SystemProbe = pkgconfigsetup.SystemProbe
)

// IsAutoconfigEnabled is alias for model.IsAutoconfigEnabled
func IsAutoconfigEnabled() bool {
	return env.IsAutoconfigEnabled(Datadog())
}

// Aliases for config overrides
var (
	AddOverride     = model.AddOverride
	AddOverrides    = model.AddOverrides
	AddOverrideFunc = model.AddOverrideFunc
)

// LoggerName Alias
type LoggerName = pkglogsetup.LoggerName

// Aliases for  logs
var (
	NewLogWriter               = pkglogsetup.NewLogWriter
	NewTLSHandshakeErrorWriter = pkglogsetup.NewTLSHandshakeErrorWriter
)

// SetupLogger Alias using Datadog config
func SetupLogger(loggerName LoggerName, logLevel, logFile, syslogURI string, syslogRFC, logToConsole, jsonFormat bool) error {
	return pkglogsetup.SetupLogger(loggerName, logLevel, logFile, syslogURI, syslogRFC, logToConsole, jsonFormat, Datadog())
}

// SetupJMXLogger Alias using Datadog config
func SetupJMXLogger(logFile, syslogURI string, syslogRFC, logToConsole, jsonFormat bool) error {
	return pkglogsetup.SetupJMXLogger(logFile, syslogURI, syslogRFC, logToConsole, jsonFormat, Datadog())
}

// GetSyslogURI Alias using Datadog config
func GetSyslogURI() string {
	return pkglogsetup.GetSyslogURI(Datadog())
}

// SetupDogstatsdLogger Alias using Datadog config
func SetupDogstatsdLogger(logFile string) (slog.LoggerInterface, error) {
	return pkglogsetup.SetupDogstatsdLogger(logFile, Datadog())
}

// IsCloudProviderEnabled Alias using Datadog config
func IsCloudProviderEnabled(cloudProvider string) bool {
	return pkgconfigsetup.IsCloudProviderEnabled(cloudProvider, Datadog())
}

// GetIPCAddress Alias using Datadog config
func GetIPCAddress() (string, error) {
	return pkgconfigsetup.GetIPCAddress(Datadog())
}

// Datatype Aliases
const (
	Metrics = pkgconfigsetup.Metrics
	Traces  = pkgconfigsetup.Traces
	Logs    = pkgconfigsetup.Logs
)

// Aliases for config defaults
const (
	DefaultForwarderRecoveryInterval         = pkgconfigsetup.DefaultForwarderRecoveryInterval
	DefaultAPIKeyValidationInterval          = pkgconfigsetup.DefaultAPIKeyValidationInterval
	DefaultBatchWait                         = pkgconfigsetup.DefaultBatchWait
	DefaultInputChanSize                     = pkgconfigsetup.DefaultInputChanSize
	DefaultBatchMaxConcurrentSend            = pkgconfigsetup.DefaultBatchMaxConcurrentSend
	DefaultBatchMaxContentSize               = pkgconfigsetup.DefaultBatchMaxContentSize
	DefaultLogsSenderBackoffRecoveryInterval = pkgconfigsetup.DefaultLogsSenderBackoffRecoveryInterval
	DefaultLogsSenderBackoffMax              = pkgconfigsetup.DefaultLogsSenderBackoffMax
	DefaultLogsSenderBackoffFactor           = pkgconfigsetup.DefaultLogsSenderBackoffFactor
	DefaultLogsSenderBackoffBase             = pkgconfigsetup.DefaultLogsSenderBackoffBase
	DefaultBatchMaxSize                      = pkgconfigsetup.DefaultBatchMaxSize
	DefaultNumWorkers                        = pkgconfigsetup.DefaultNumWorkers
	MaxNumWorkers                            = pkgconfigsetup.MaxNumWorkers
	DefaultSite                              = pkgconfigsetup.DefaultSite
	OTLPTracePort                            = pkgconfigsetup.OTLPTracePort
	DefaultAuditorTTL                        = pkgconfigsetup.DefaultAuditorTTL
	DefaultMaxMessageSizeBytes               = pkgconfigsetup.DefaultMaxMessageSizeBytes
	DefaultProcessEntityStreamPort           = pkgconfigsetup.DefaultProcessEntityStreamPort
	DefaultProcessEventsCheckInterval        = pkgconfigsetup.DefaultProcessEventsCheckInterval
	DefaultProcessEventsMinCheckInterval     = pkgconfigsetup.DefaultProcessEventsMinCheckInterval
	ProcessMaxPerMessageLimit                = pkgconfigsetup.ProcessMaxPerMessageLimit
	DefaultProcessMaxPerMessage              = pkgconfigsetup.DefaultProcessMaxPerMessage
	ProcessMaxMessageBytesLimit              = pkgconfigsetup.ProcessMaxMessageBytesLimit
	DefaultProcessDiscoveryHintFrequency     = pkgconfigsetup.DefaultProcessDiscoveryHintFrequency
	DefaultProcessMaxMessageBytes            = pkgconfigsetup.DefaultProcessMaxMessageBytes
	DefaultProcessExpVarPort                 = pkgconfigsetup.DefaultProcessExpVarPort
	DefaultProcessQueueBytes                 = pkgconfigsetup.DefaultProcessQueueBytes
	DefaultProcessQueueSize                  = pkgconfigsetup.DefaultProcessQueueSize
	DefaultProcessRTQueueSize                = pkgconfigsetup.DefaultProcessRTQueueSize
	DefaultRuntimePoliciesDir                = pkgconfigsetup.DefaultRuntimePoliciesDir
	DefaultGRPCConnectionTimeoutSecs         = pkgconfigsetup.DefaultGRPCConnectionTimeoutSecs
	DefaultProcessEndpoint                   = pkgconfigsetup.DefaultProcessEndpoint
	DefaultProcessEventsEndpoint             = pkgconfigsetup.DefaultProcessEventsEndpoint
)

type (
	// ConfigurationProviders Alias
	ConfigurationProviders = pkgconfigsetup.ConfigurationProviders
	// Listeners Alias
	Listeners = pkgconfigsetup.Listeners
)

// GetObsPipelineURL Alias using Datadog config
func GetObsPipelineURL(datatype pkgconfigsetup.DataType) (string, error) {
	return pkgconfigsetup.GetObsPipelineURL(datatype, Datadog())
}

// LoadCustom Alias
func LoadCustom(config model.Config, additionalKnownEnvVars []string) error {
	return pkgconfigsetup.LoadCustom(config, additionalKnownEnvVars)
}

// LoadDatadogCustom Alias
func LoadDatadogCustom(config model.Config, origin string, secretResolver optional.Option[secrets.Component], additionalKnownEnvVars []string) (*model.Warnings, error) {
	return pkgconfigsetup.LoadDatadogCustom(config, origin, secretResolver, additionalKnownEnvVars)
}

// GetValidHostAliases Alias using Datadog config
func GetValidHostAliases(ctx context.Context) ([]string, error) {
	return pkgconfigsetup.GetValidHostAliases(ctx, Datadog())
}

// IsCLCRunner Alias using Datadog config
func IsCLCRunner() bool {
	return pkgconfigsetup.IsCLCRunner(Datadog())
}

// GetBindHostFromConfig Alias using Datadog config
func GetBindHostFromConfig(config model.Reader) string {
	return pkgconfigsetup.GetBindHostFromConfig(config)
}

// GetBindHost Alias using Datadog config
func GetBindHost() string {
	return pkgconfigsetup.GetBindHost(Datadog())
}

var (
	// IsRemoteConfigEnabled Alias
	IsRemoteConfigEnabled = pkgconfigsetup.IsRemoteConfigEnabled
	// StartTime Alias
	StartTime = pkgconfigsetup.StartTime
	// StandardJMXIntegrations Alias
	StandardJMXIntegrations = pkgconfigsetup.StandardJMXIntegrations
	// SetupOTLP Alias
	SetupOTLP = pkgconfigsetup.OTLP
	// InitSystemProbeConfig Alias
	InitSystemProbeConfig = pkgconfigsetup.InitSystemProbeConfig
	// InitConfig Alias
	InitConfig = pkgconfigsetup.InitConfig

	// GetRemoteConfigurationAllowedIntegrations Alias
	GetRemoteConfigurationAllowedIntegrations = pkgconfigsetup.GetRemoteConfigurationAllowedIntegrations
	// LoadProxyFromEnv Alias
	LoadProxyFromEnv = pkgconfigsetup.LoadProxyFromEnv

	// GetIPCPort Alias
	GetIPCPort = pkgconfigsetup.GetIPCPort
)

// LoadWithoutSecret Alias using Datadog config
func LoadWithoutSecret() (*model.Warnings, error) {
	return pkgconfigsetup.LoadDatadogCustom(Datadog(), "datadog.yaml", optional.NewNoneOption[secrets.Component](), SystemProbe().GetEnvVars())
}

// GetProcessAPIAddressPort Alias using Datadog config
func GetProcessAPIAddressPort() (string, error) {
	return pkgconfigsetup.GetProcessAPIAddressPort(Datadog())
}

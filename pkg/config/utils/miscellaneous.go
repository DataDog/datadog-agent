// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils offers a number of high level helpers to work with the configuration
package utils

import (
	"path/filepath"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgfips "github.com/DataDog/datadog-agent/pkg/fips"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConfFileDirectory returns the absolute path to the folder containing the config
// file used to populate the registry
func ConfFileDirectory(c pkgconfigmodel.Reader) string {
	return filepath.Dir(c.ConfigFileUsed())
}

// SetLogLevel validates and sets the "log_level" setting in the configuration. The logger will automatically react to this configuration change.
// It takes a `level` string representing the desired log level and a `source` model.Source indicating where the new level came from (CLI, Remote Config, ...).
// It returns an error if the log level is invalid
func SetLogLevel(level string, config pkgconfigmodel.Writer, source pkgconfigmodel.Source) error {
	seelogLogLevel, err := log.ValidateLogLevel(level)
	if err != nil {
		return err
	}
	// Logger subscribe to config changes to automatically apply new log_level value
	config.Set("log_level", seelogLogLevel, source)
	return nil
}

// IsCoreAgentEnabled checks if the Agent is able to send the payloads it and other Agents need to function with
func IsCoreAgentEnabled(cfg pkgconfigmodel.Reader) bool {
	if !cfg.GetBool("core_agent.enabled") {
		return false
	}

	// core_agent.enabled can be true but the following payloads if set to false means
	// core_agent is disabled
	if !cfg.GetBool("enable_payloads.events") &&
		!cfg.GetBool("enable_payloads.series") &&
		!cfg.GetBool("enable_payloads.service_checks") &&
		!cfg.GetBool("enable_payloads.sketches") {
		return false
	}

	return true
}

// IsAPMEnabled checks if APM is enabled or if Error Tracking standalone is enabled, simplifying the setup for
// Error Tracking standalone only via the apm_config.error_tracking_standalone.enabled option instead of requiring
// to enable also apm_config.enabled.
func IsAPMEnabled(cfg pkgconfigmodel.Reader) bool {
	return cfg.GetBool("apm_config.enabled") ||
		cfg.GetBool("apm_config.error_tracking_standalone.enabled")
}

// IsRemoteConfigEnabled returns true if Remote Configuration should be enabled
func IsRemoteConfigEnabled(cfg pkgconfigmodel.Reader) bool {
	// Disable Remote Config for GovCloud if it's not explicitly enabled
	if IsFed(cfg) && !cfg.IsConfigured("remote_configuration.enabled") {
		return false
	}
	return cfg.GetBool("remote_configuration.enabled")
}

// IsFed returns true if the Agent is running in a gov environment
func IsFed(cfg pkgconfigmodel.Reader) bool {
	isFipsAgent, _ := pkgfips.Enabled()
	return cfg.GetBool("fips.enabled") || isFipsAgent || cfg.GetString("site") == "ddog-gov.com" || cfg.GetString("dd_url") == "https://app.ddog-gov.com"
}

// IsCloudProviderEnabled checks the cloud provider family provided in
// pkg/util/<cloud_provider>.go against the value for cloud_provider: on the
// global config object Datadog
func IsCloudProviderEnabled(cloudProviderName string, config pkgconfigmodel.Reader) bool {
	cloudProviderFromConfig := config.GetStringSlice("cloud_provider_metadata")

	for _, cloudName := range cloudProviderFromConfig {
		if strings.EqualFold(cloudName, cloudProviderName) {
			log.Debugf("cloud_provider_metadata is set to %s in agent configuration, trying endpoints for %s Cloud Provider",
				cloudProviderFromConfig,
				cloudProviderName)
			return true
		}
	}

	log.Debugf("cloud_provider_metadata is set to %s in agent configuration, skipping %s Cloud Provider",
		cloudProviderFromConfig,
		cloudProviderName)
	return false
}

// GetBindHost returns `bind_host` variable or default value
// Not using `config.BindEnvAndSetDefault` as some processes need to know
// if value was default one or not (e.g. trace-agent)
func GetBindHost(cfg pkgconfigmodel.Reader) string {
	if cfg.IsSet("bind_host") {
		return cfg.GetString("bind_host")
	}
	return "localhost"
}

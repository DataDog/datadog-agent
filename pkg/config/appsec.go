// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

// Default configuration values
const (
	DefaultAppSecEnabled        = true
	DefaultAppSecDDUrl          = ""
	DefaultAppSecMaxPayloadSize = 5 * 1024 * 1024

	DefaultAppSecObfuscationKeyRegexps   = `(?i)(p(ass)?w(((or)?d))|(phrase))|(secret)|(authorization)|(api_?key)|((access_?)?token)`
	DefaultAppSecObfuscationValueRegexps = ``
)

// AppSec configuration key entries. Exported to be reused from other packages.
const (
	AppSecConfigKeyObfuscationParameterKeyRegexp   = "appsec_config.obfuscation.parameter_key_regexp"
	AppSecConfigKeyObfuscationParameterValueRegexp = "appsec_config.obfuscation.parameter_value_regexp"
)

// setupAppSec initializes the configuration values of the appsec agent.
func setupAppSec(cfg Config) {
	cfg.BindEnvAndSetDefault("appsec_config.enabled", DefaultAppSecEnabled, "DD_APPSEC_ENABLED")
	cfg.BindEnvAndSetDefault("appsec_config.appsec_dd_url", DefaultAppSecDDUrl, "DD_APPSEC_DD_URL")
	cfg.BindEnvAndSetDefault("appsec_config.max_payload_size", DefaultAppSecMaxPayloadSize, "DD_APPSEC_MAX_PAYLOAD_SIZE")
	cfg.BindEnvAndSetDefault(AppSecConfigKeyObfuscationParameterKeyRegexp, DefaultAppSecObfuscationKeyRegexps, "DD_APPSEC_OBFUSCATION_PARAMETER_KEY_REGEXP")
	cfg.BindEnvAndSetDefault(AppSecConfigKeyObfuscationParameterValueRegexp, DefaultAppSecObfuscationValueRegexps, "DD_APPSEC_OBFUSCATION_PARAMETER_VALUE_REGEXP")
}

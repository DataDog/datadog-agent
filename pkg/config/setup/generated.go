// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// NOTE! This is a generated file, do not modify it. Created by `dda inv schema.codegen`

package setup

//
// The following code is generated from the schema and should never be manually edited
//

type delegatedAuthConfig struct {
	apiKeyPath        string
	delegatedAuthPath string
	description       string
}

// delegatedAuthKeys list all the "delegated_auth" configuration section.
// This list is used to fully initialize authentication through cloud provider instead of API key
var delegatedAuthKeys = []delegatedAuthConfig{

	{
		apiKeyPath:        "remote_configuration.api_key",
		delegatedAuthPath: "remote_configuration.delegated_auth",
		description:       "remote_configuration",
	},

	{
		apiKeyPath:        "logs_config.api_key",
		delegatedAuthPath: "logs_config.delegated_auth",
		description:       "logs_config",
	},

	{
		apiKeyPath:        "api_key",
		delegatedAuthPath: "delegated_auth",
		description:       "global",
	},

	{
		apiKeyPath:        "evp_proxy_config.api_key",
		delegatedAuthPath: "evp_proxy_config.delegated_auth",
		description:       "evp_proxy_config",
	},

	{
		apiKeyPath:        "ol_proxy_config.api_key",
		delegatedAuthPath: "ol_proxy_config.delegated_auth",
		description:       "ol_proxy_config",
	},
}

// Constants generated from settings tagged with a `generate_const:<name>` label.
// Each constant's value is the default of its associated setting.
const (
	DefaultAPIKeyValidationInterval               = 60
	DefaultAuditorTTL                             = 23
	DefaultBatchMaxConcurrentSend                 = 0
	DefaultBatchMaxContentSize                    = 5000000
	DefaultBatchMaxSize                           = 1000
	DefaultBatchWait                              = float64(5)
	DefaultCompressorKind                         = "zstd"
	DefaultForwarderRecoveryInterval              = 2
	DefaultInputChanSize                          = 100
	DefaultLogCompressionKind                     = "zstd"
	DefaultLogsSenderBackoffBase                  = float64(1)
	DefaultLogsSenderBackoffFactor                = float64(2)
	DefaultLogsSenderBackoffMax                   = float64(120)
	DefaultMaxMessageSizeBytes                    = 900000
	DefaultNetworkPathMaxTTL                      = 30
	DefaultNetworkPathStaticPathE2eQueries        = 50
	DefaultNetworkPathStaticPathTracerouteQueries = 3
	DefaultNetworkPathTimeout                     = 1000
	DefaultSecurityAgentCmdPort                   = 5010
	DefaultSite                                   = "datadoghq.com"
	DefaultZstdCompressionLevel                   = 1
	defaultBTFOutputDir                           = "/var/tmp/datadog-agent/system-probe/btf"
	defaultConnsMessageBatchSize                  = 600
	defaultDynamicInstrumentationDebugInfoDir     = "${run_path}/system-probe/dynamic-instrumentation/decompressed-debug-info"
	defaultEnvoyPath                              = "/bin/envoy"
	defaultOffsetThreshold                        = int64(400)
	defaultRuntimeCompilerOutputDir               = "/var/tmp/datadog-agent/system-probe/build"
)

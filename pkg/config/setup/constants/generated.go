// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// NOTE! This is a generated file, do not modify it. Created by `dda inv schema.codegen`

package constants

//
// The following code is generated from the schema and should never be manually edited
//

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
)

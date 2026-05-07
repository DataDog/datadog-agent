// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

// Default values for configuration keys that are used as fallbacks when a
// configured value fails validation.  Keeping them here allows comp/ packages
// to import them without pulling in pkg/config/setup.
const (
	// DefaultSite is the default site the Agent sends data to.
	DefaultSite = "datadoghq.com"

	// DefaultAPIKeyValidationInterval is the default interval of api key validation checks.
	DefaultAPIKeyValidationInterval = 60

	// DefaultForwarderRecoveryInterval is the default recovery interval, also used if the
	// user-provided value is invalid.
	DefaultForwarderRecoveryInterval = 2

	// DefaultBatchWait is the default HTTP batch wait in seconds for logs.
	DefaultBatchWait = 5.0

	// DefaultBatchMaxConcurrentSend is the default HTTP batch max concurrent send for logs.
	DefaultBatchMaxConcurrentSend = 0

	// DefaultBatchMaxSize is the default HTTP batch max size.
	DefaultBatchMaxSize = 1000

	// DefaultInputChanSize is the default event-platform input channel size.
	DefaultInputChanSize = 100

	// DefaultBatchMaxContentSize is the default HTTP batch max content size.
	DefaultBatchMaxContentSize = 5000000

	// DefaultCompressorKind is the default serializer compressor kind.
	DefaultCompressorKind = "zstd"

	// DefaultLogCompressionKind is the default logs compression kind.
	DefaultLogCompressionKind = "zstd"

	// DefaultZstdCompressionLevel is the default zstd compression level.
	DefaultZstdCompressionLevel = 1

	// DefaultLogsSenderBackoffFactor is the default logs sender exponential backoff factor.
	DefaultLogsSenderBackoffFactor = 2.0

	// DefaultLogsSenderBackoffBase is the default logs sender exponential backoff base.
	DefaultLogsSenderBackoffBase = 1.0

	// DefaultLogsSenderBackoffMax is the default logs sender exponential backoff max.
	DefaultLogsSenderBackoffMax = 120.0

	// DefaultLogsSenderBackoffRecoveryInterval is the default logs sender backoff recovery interval.
	DefaultLogsSenderBackoffRecoveryInterval = 2

	// DefaultProcessQueueBytes is the default amount of process-agent check data (in bytes)
	// that can be buffered in memory.
	DefaultProcessQueueBytes = 60 * 1000 * 1000

	// DefaultProcessExpVarPort is the default port used by the process-agent expvar server.
	DefaultProcessExpVarPort = 6062

	// Metrics identifies the metrics payload type.
	Metrics = "metrics"
)

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import "github.com/DataDog/datadog-agent/pkg/config/setup/constants"

// These are re-exported here because `opentelemetry-collector-contrib/pkg/datadog/agentcomponents` still reaches
// for them through `pkgconfigsetup`. Once OTEL is updated they will be removed.
const (
	// DefaultAuditorTTL is an alias of constants.DefaultAuditorTTL
	DefaultAuditorTTL = constants.DefaultAuditorTTL
	// DefaultBatchMaxContentSize is an alias of constants.DefaultBatchMaxContentSize
	DefaultBatchMaxContentSize = constants.DefaultBatchMaxContentSize
	// DefaultBatchMaxSize is an alias of constants.DefaultBatchMaxSize
	DefaultBatchMaxSize = constants.DefaultBatchMaxSize
	// DefaultForwarderRecoveryInterval is an alias of constants.DefaultForwarderRecoveryInterval
	DefaultForwarderRecoveryInterval = constants.DefaultForwarderRecoveryInterval
	// DefaultInputChanSize is an alias of constants.DefaultInputChanSize
	DefaultInputChanSize = constants.DefaultInputChanSize
	// DefaultLogsSenderBackoffBase is an alias of constants.DefaultLogsSenderBackoffBase
	DefaultLogsSenderBackoffBase = constants.DefaultLogsSenderBackoffBase
	// DefaultLogsSenderBackoffFactor is an alias of constants.DefaultLogsSenderBackoffFactor
	DefaultLogsSenderBackoffFactor = constants.DefaultLogsSenderBackoffFactor
	// DefaultLogsSenderBackoffMax is an alias of constants.DefaultLogsSenderBackoffMax
	DefaultLogsSenderBackoffMax = constants.DefaultLogsSenderBackoffMax
	// DefaultMaxMessageSizeBytes is an alias of constants.DefaultMaxMessageSizeBytes
	DefaultMaxMessageSizeBytes = constants.DefaultMaxMessageSizeBytes
)

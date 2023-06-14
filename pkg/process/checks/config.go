// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// getMaxBatchSize returns the maximum number of items (processes, containers, process_discoveries) in a check payload
var getMaxBatchSize = func(config ddconfig.ConfigReader) int {
	return ensureValidMaxBatchSize(config.GetInt("process_config.max_per_message"))
}

func ensureValidMaxBatchSize(batchSize int) int {
	if batchSize <= 0 || batchSize > ddconfig.DefaultProcessMaxPerMessage {
		log.Warnf("Invalid max item count per message (%d), using default value of %d", batchSize, ddconfig.DefaultProcessMaxPerMessage)
		return ddconfig.DefaultProcessMaxPerMessage
	}
	return batchSize
}

// getMaxBatchSize returns the maximum number of bytes in a check payload
var getMaxBatchBytes = func(config ddconfig.ConfigReader) int {
	return ensureValidMaxBatchBytes(config.GetInt("process_config.max_message_bytes"))
}

func ensureValidMaxBatchBytes(batchBytes int) int {
	if batchBytes <= 0 || batchBytes > ddconfig.DefaultProcessMaxMessageBytes {
		log.Warnf("Invalid max byte size per message (%d), using default value of %d", batchBytes, ddconfig.DefaultProcessMaxMessageBytes)
		return ddconfig.DefaultProcessMaxMessageBytes
	}
	return batchBytes
}

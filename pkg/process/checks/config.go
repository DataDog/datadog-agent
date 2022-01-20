// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"sync"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// maxBatchSizeOnce is used to only read the process_config.max_per_message config once and set maxBatchSize
	maxBatchSizeOnce sync.Once
	maxBatchSize     int

	// maxCtrProcsBatchSizeOnce is used to only read the process_config.max_ctr_procs_per_message config once and set maxCtrProcsBatchSize
	maxCtrProcsBatchSizeOnce sync.Once
	maxCtrProcsBatchSize     int
)

// getMaxBatchSize returns the maximum number of items (processes, containers, process_discoveries) in a check payload
func getMaxBatchSize() int {
	maxBatchSizeOnce.Do(func() {
		batchSize := ddconfig.Datadog.GetInt("process_config.max_per_message")
		if batchSize <= 0 {
			batchSize = ddconfig.DefaultProcessMaxPerMessage
			log.Warnf("Invalid item count per message (<= 0), using default value of %d", ddconfig.DefaultProcessMaxPerMessage)

		} else if batchSize > ddconfig.DefaultProcessMaxPerMessage {
			batchSize = ddconfig.DefaultProcessMaxPerMessage
			log.Warnf("Overriding the configured max of item count per message because it exceeds maximum limit of %d", ddconfig.DefaultProcessMaxPerMessage)
		}

		maxBatchSize = batchSize
	})

	return maxBatchSize
}

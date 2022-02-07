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
	maxBatchSizeLogOnce         sync.Once
	maxCtrProcsBatchSizeLogOnce sync.Once
)

// getMaxBatchSize returns the maximum number of items (processes, containers, process_discoveries) in a check payload
func getMaxBatchSize() int {
	batchSize := ddconfig.Datadog.GetInt("process_config.max_per_message")

	if batchSize <= 0 {
		batchSize = ddconfig.DefaultProcessMaxPerMessage
		maxBatchSizeLogOnce.Do(func() {
			log.Warnf("Invalid item count per message (<= 0), using default value of %d", ddconfig.DefaultProcessMaxPerMessage)
		})

	} else if batchSize > ddconfig.DefaultProcessMaxPerMessage {
		batchSize = ddconfig.DefaultProcessMaxPerMessage
		maxBatchSizeLogOnce.Do(func() {
			log.Warnf("Overriding the configured max of item count per message because it exceeds maximum limit of %d", ddconfig.DefaultProcessMaxPerMessage)
		})
	}

	return batchSize
}

// getMaxCtrProcsBatchSize returns the maximum number of processes belonging to a container in a check payload
func getMaxCtrProcsBatchSize() int {
	ctrProcsBatchSize := ddconfig.Datadog.GetInt("process_config.max_ctr_procs_per_message")

	if ctrProcsBatchSize <= 0 {
		ctrProcsBatchSize = ddconfig.DefaultProcessMaxCtrProcsPerMessage
		maxCtrProcsBatchSizeLogOnce.Do(func() {
			log.Warnf("Invalid max container processes count per message (<= 0), using default value of %d", ddconfig.DefaultProcessMaxCtrProcsPerMessage)
		})

	} else if ctrProcsBatchSize > ddconfig.ProcessMaxCtrProcsPerMessageLimit {
		ctrProcsBatchSize = ddconfig.DefaultProcessMaxCtrProcsPerMessage
		maxCtrProcsBatchSizeLogOnce.Do(func() {
			log.Warnf("Overriding the configured max of container processes count per message because it exceeds maximum limit of %d", ddconfig.ProcessMaxCtrProcsPerMessageLimit)
		})
	}

	return ctrProcsBatchSize
}

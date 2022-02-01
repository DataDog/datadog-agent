// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import ddconfig "github.com/DataDog/datadog-agent/pkg/config"

var (
	// MaxBatchSize is the max number of items (processes, containers, process-discoveries) in a RunResult
	MaxBatchSize = ddconfig.DefaultProcessMaxPerMessage

	// MaxCtrProcsBatchSize is the maximum number of processes belonging to a container per message
	MaxCtrProcsBatchSize = ddconfig.DefaultProcessMaxCtrProcsPerMessage
)

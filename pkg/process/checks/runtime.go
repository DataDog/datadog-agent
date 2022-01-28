package checks

import ddconfig "github.com/DataDog/datadog-agent/pkg/config"

var (
	// MaxBatchSize is the max number of items (processes, containers, processs-discoveries) in a RunResult
	MaxBatchSize = ddconfig.DefaultProcessMaxPerMessage

	// MaxCtrProcsBatchSize is the maximum number of processes belonging to a container per message
	MaxCtrProcsBatchSize = ddconfig.DefaultProcessMaxCtrProcsPerMessage
)

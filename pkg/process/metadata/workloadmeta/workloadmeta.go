// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/languagedetection"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// WorkloadMetaExtractor handles enriching processes with
type WorkloadMetaExtractor struct{}

// NewWorkloadMetaExtractor constructs the WorkloadMetaExtractor.
func NewWorkloadMetaExtractor() *WorkloadMetaExtractor {
	return &WorkloadMetaExtractor{}
}

func (w *WorkloadMetaExtractor) Extract(procs map[int32]*procutil.Process) {
	procsSlice := make([]*languagedetection.Process, 0, len(procs))
	for _, proc := range procs {
		procsSlice = append(procsSlice, &languagedetection.Process{
			Pid:     proc.Pid,
			Cmdline: proc.Cmdline,
		})
	}

	languages := languagedetection.DetectLanguage(procsSlice)
	for i, proc := range procsSlice {
		lang := languages[i]
		if proc, ok := procs[proc.Pid]; ok {
			proc.Language = lang
		}
	}
}

// Enabled returns wheither or not the extractor should be enabled
func Enabled(ddconfig config.ConfigReader) bool {
	return ddconfig.GetBool("process_config.language_detection.enabled")
}

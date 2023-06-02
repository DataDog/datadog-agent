package workloadmeta

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/languagedetection"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

type WorkloadMetaExtractor struct {
	enabled bool
}

func NewWorkloadMetaExtractor(ddconfig config.ConfigReader) *WorkloadMetaExtractor {
	return &WorkloadMetaExtractor{
		enabled: ddconfig.GetBool("process_config.language_detection.enabled"),
	}
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

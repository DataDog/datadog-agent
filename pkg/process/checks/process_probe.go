package checks

import (
	"runtime"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	processProbe     procutil.Probe
	processProbeOnce sync.Once
	legacyProbe      procutil.Probe
)

func getProcessProbe(cfg *config.AgentConfig) procutil.Probe {
	processProbeOnce.Do(func() {
		if runtime.GOOS == "windows" {
			if !cfg.Windows.UsePerfCounters {
				log.Info("Using toolhelp API probe for process data collection")
				processProbe = legacyProbe
				return
			}
			log.Info("Using perf counters probe for process data collection")
		}
		processProbe = procutil.NewProcessProbe()
	})
	return processProbe
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"runtime"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	processProbe        procutil.Probe
	processProbeOnce    sync.Once
	defaultWindowsProbe procutil.Probe
)

func getProcessProbe() procutil.Probe {
	processProbeOnce.Do(func() {
		if runtime.GOOS == "windows" {
			if !config.Datadog.GetBool("process_config.windows.use_perf_counters") {
				log.Info("Using toolhelp API probe for process data collection")
				processProbe = defaultWindowsProbe
				return
			}
			log.Info("Using perf counters probe for process data collection")
		}
		processProbe = procutil.NewProcessProbe()
	})
	return processProbe
}

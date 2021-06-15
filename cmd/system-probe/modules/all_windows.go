// +build windows

package modules

import (
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"golang.org/x/sys/windows/svc/eventlog"
)

// All System Probe modules should register their factories here
var All = []module.Factory{
	NetworkTracer,
}

const (
	msgSysprobeRestartInactivity = 0x8000000f
)

func inactivityEventLog(duration time.Duration) {
	elog, err := eventlog.Open(config.ServiceName)
	if err != nil {
		return
	}
	defer elog.Close()
	elog.Warning(msgSysprobeRestartInactivity, duration.String())
}

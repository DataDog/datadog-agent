// +build windows

package modules

import (
	"time"

	"golang.org/x/sys/windows/svc/eventlog"
)

const (
	msgSysprobeRestartInactivity = 0x8000000f
	serviceName                  = "datadog-system-probe"
)

func inactivityEventLog(duration time.Duration) {
	elog, err := eventlog.Open(serviceName)
	if err != nil {
		return
	}
	defer elog.Close()
	elog.Warning(msgSysprobeRestartInactivity, duration.String())
}

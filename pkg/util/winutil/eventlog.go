// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package winutil

import (
	"golang.org/x/sys/windows/svc/eventlog"
)

// LogEventViewer will open the event viewer API and log a single message
// to the event viewer.  The string identified in the msgnum parameter
// must exist in the application's message catalog
// go log api only provides for a single argument to be passed, so can
// only include one positional argument
func LogEventViewer(servicename string, msgnum uint32, arg string) {
	elog, err := eventlog.Open(servicename)
	if err != nil {
		return
	}
	defer elog.Close()
	switch msgnum & 0xF0000000 {
	case 0x40000000:
		// Info level message
		_ = elog.Info(msgnum, arg)
	case 0x80000000:
		// warning level message
		_ = elog.Warning(msgnum, arg)
	case 0xC0000000:
		// error level message
		_ = elog.Error(msgnum, arg)
	}

}

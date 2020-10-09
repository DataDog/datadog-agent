// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package watchdog

import (
	"fmt"
	"runtime"

	metricsClient "github.com/DataDog/datadog-agent/pkg/trace/exportable/metrics/client"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const shortErrMsgLen = 17 // 20 char max with tailing "..."

// shortMsg shortens the length of error message to avoid having high
// cardinality on "err:" tags
func shortErrMsg(msg string) string {
	if len(msg) <= shortErrMsgLen {
		return msg
	}
	return msg[:shortErrMsgLen] + "..."
}

// LogOnPanic catches panics and logs them on the fly. It also flushes
// the log file, ensuring the message appears. Then it propagates the panic
// so that the program flow remains unchanged.
func LogOnPanic() {
	if err := recover(); err != nil {
		// Full print of the trace in the logs
		buf := make([]byte, 4096)
		length := runtime.Stack(buf, false)
		stacktrace := string(buf[:length])
		errMsg := fmt.Sprintf("%v", err)
		logMsg := "Unexpected panic: " + errMsg + "\n" + stacktrace

		metricsClient.Gauge("datadog.trace_agent.panic", 1, []string{
			"err:" + shortErrMsg(errMsg),
		}, 1)

		log.Error(logMsg)
		log.Flush()

		panic(err)
	}
}

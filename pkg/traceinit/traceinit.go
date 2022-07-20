// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceinit

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var initTraceFile *os.File
var initTraceFileOnce sync.Once

func TraceFunction(text string) {
	initTraceFileOnce.Do(func() {
		initTraceFile, _ = os.OpenFile("C:\\ProgramData\\Datadog\\logs\\traceinit.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	})
	initTraceFile.WriteString(fmt.Sprintf("%v %v\n", time.Now(), text))
	initTraceFile.Sync()
}

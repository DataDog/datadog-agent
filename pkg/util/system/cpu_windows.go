// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package system

import (
	"context"
	"runtime"
)
import "github.com/DataDog/datadog-agent/pkg/traceinit"


func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\util\system\cpu_windows.go 13`)
	// TODO: Implement proper CPU Count for Windows too
	// As runtime.NumCPU() supports Windows CPU Affinity
	cpuInfoFunc = func(context.Context, bool) (int, error) {
		return runtime.NumCPU(), nil
	}
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\util\system\cpu_windows.go 18`)
}
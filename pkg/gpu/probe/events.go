// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"time"

	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/probe/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// logLimitProbe is used to limit the number of times we log messages about streams and cuda events, as that can be very verbose
var logLimitProbe = log.NewLogLimit(20, 10*time.Minute)

func getPidTidFromHeader(header *gpuebpf.CudaEventHeader) (uint32, uint32) {
	tid := uint32(header.Pid_tgid & 0xFFFFFFFF)
	pid := uint32(header.Pid_tgid >> 32)
	return pid, tid
}

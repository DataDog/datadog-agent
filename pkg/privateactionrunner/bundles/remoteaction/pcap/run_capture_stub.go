// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !(linux && pcap && cgo)

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"time"
)

// doCapture is a stub for platforms that do not support eBPF-based packet
// capture (i.e. anything other than linux+pcap+cgo). Returns zero stats
// and nil error to allow unit tests and non-Linux builds to pass input
// validation tests. The PAR binary is only built for Linux, so this stub
// is never used in production.
func doCapture(_ context.Context, _ RunCaptureInputs) (int, int64, time.Duration, string, error) {
	return 0, 0, 0, "", nil
}

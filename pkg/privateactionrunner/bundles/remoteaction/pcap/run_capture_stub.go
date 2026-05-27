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
// capture (i.e. anything other than linux+pcap+cgo). It returns zero stats
// and no error so that the caller returns a valid RunCaptureResult with
// empty capture data rather than a hard failure.
//
// Note: deployments targeting real capture must use the linux+pcap+cgo build.
func doCapture(_ context.Context, _ RunCaptureInputs) (int, int64, time.Duration, error) {
	return 0, 0, 0, nil
}

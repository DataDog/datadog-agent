// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !unix

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"errors"
	"time"
)

// stubCaptureTrigger reports that packet capture is unavailable: the
// packet_capture system-probe module is only reachable over a unix domain
// socket.
type stubCaptureTrigger struct{}

func newCaptureTrigger() captureTrigger {
	return &stubCaptureTrigger{}
}

// Capture always fails on non-unix platforms.
func (*stubCaptureTrigger) Capture(_ context.Context, _ RunCaptureInputs) (int, int64, time.Duration, string, error) {
	return 0, 0, 0, "", errors.New("packet capture is not supported on this platform")
}

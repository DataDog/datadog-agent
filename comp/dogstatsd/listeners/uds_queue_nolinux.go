// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package listeners

import (
	"net"
	"time"
)

type udsQueueTelemetry struct{}

func startUDSQueueTelemetry(_ *net.UnixConn, _ time.Duration, _ *TelemetryStore) (*udsQueueTelemetry, error) {
	return nil, nil
}

func (*udsQueueTelemetry) close() {}

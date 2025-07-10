// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

package udp

import (
	"fmt"
)

func (*UDPv4) newTracerouteDriver() (*udpDriver, error) {
	return nil, fmt.Errorf("UDP getTracerouteDriver is not supported on this platform")
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

package tcp

import (
	"fmt"
)

func (*TCPv4) newTracerouteDriver() (*tcpDriver, error) {
	return nil, fmt.Errorf("TCP getTracerouteDriver is not yet supported on this platform")
}

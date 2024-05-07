// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package failure

import (
	"github.com/DataDog/datadog-agent/pkg/network"
)

// MatchFailedConn increments the failed connection counters for a given connection
func MatchFailedConn(_ *network.ConnectionStats, _ *FailedConns) {
	// Not implemented on Windows
}

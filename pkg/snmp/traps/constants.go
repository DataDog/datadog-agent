// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package traps

import "time"

const (
	defaultPort        = uint16(162) // Standard UDP port for traps.
	defaultStopTimeout = 5 * time.Second
	packetsChanSize    = 100
)

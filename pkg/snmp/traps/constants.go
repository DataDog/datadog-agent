// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traps

const (
	defaultPort        = uint16(162) // Standard UDP port for traps.
	defaultStopTimeout = 5
	packetsChanSize    = 100
)

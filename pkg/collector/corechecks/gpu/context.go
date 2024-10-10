// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package gpu

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"
	sectime "github.com/DataDog/datadog-agent/pkg/security/resolvers/time"
)

type probeContext struct {
	// sender is the aggregator sender
	sender sender.Sender

	// checkDuration is the duration between checks
	checkDuration time.Duration

	// gpuDeviceMap maps GPU UUIDs to their respective device information
	gpuDeviceMap map[string]cuda.GpuDevice

	// lastCheck is the last time the check was run
	lastCheck time.Time

	// timeResolver is the time resolver to use to resolve timestamps from kernel to
	timeResolver *sectime.Resolver
}

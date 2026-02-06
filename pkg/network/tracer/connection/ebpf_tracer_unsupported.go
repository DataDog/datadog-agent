// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux || !linux_bpf

package connection

import (
	"fmt"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

// newEbpfTracer is not supported on non-Linux platforms
func newEbpfTracer(_ *config.Config, _ telemetry.Component) (Tracer, error) {
	return nil, fmt.Errorf("eBPF tracer not supported on this platform")
}

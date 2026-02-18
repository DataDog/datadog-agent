// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux || !linux_bpf

// Package connection provides network connection tracking functionality.
package connection

import (
	"errors"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

// newEbpfTracer is not supported on non-Linux platforms
//
//nolint:unused // used as stub for connection.NewTracer on non-Linux
func newEbpfTracer(_ *config.Config, _ telemetry.Component) (Tracer, error) {
	return nil, errors.New("eBPF tracer not supported on this platform")
}

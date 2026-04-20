// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && !linux_bpf

package cnm

import (
	"context"
	"errors"
)

// initTracer is a stub for builds without eBPF support.
// The receiver can still be used with a mock connectionsSource (e.g., in tests).
func (r *cnmReceiver) initTracer(_ context.Context) error {
	return errors.New("CNM receiver: eBPF not supported in this build (requires linux_bpf build tag)")
}

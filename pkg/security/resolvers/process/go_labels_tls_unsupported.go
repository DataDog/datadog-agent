// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !amd64 && !arm64

package process

import (
	"fmt"
	"runtime"

	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"
)

// extractTLSGOffset is not implemented on this architecture: recovering the
// offset means decoding the runtime's g-load sequence, which is arch-specific.
func extractTLSGOffset(_ *pfelf.File) (int32, error) {
	return 0, fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
}

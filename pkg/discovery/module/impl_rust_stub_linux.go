// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux_bpf || !cgo

package module

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
)

// errRustLibraryUnavailable is returned when the binary was built without the
// linux_bpf+cgo combination required to link libdd_discovery (for example,
// non-system-probe agent test builds or CGO_ENABLED=0 toolchain runs).
var errRustLibraryUnavailable = errors.New("libdd_discovery unavailable in this build")

// rustGetServices is the fallback stub for the Rust-backed getServices path.
// It lets the package compile in any build that reaches pkg/discovery/module
// transitively (e.g. agent test binaries that pull it in via
// cmd/system-probe/api → cmd/system-probe/modules) without requiring
// libdd_discovery.a at link time, and surfaces a clear error if reached.
func rustGetServices(_ core.Params) (*model.ServicesResponse, error) {
	return nil, errRustLibraryUnavailable
}

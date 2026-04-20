// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !cgo

package module

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
)

// errRustLibraryUnavailable is returned when the binary was built without cgo,
// so libdd_discovery cannot be called. Reaching this code path at runtime
// requires both a no-cgo build and `discovery.use_rust_library=true`; the flag
// should remain off in any such build.
var errRustLibraryUnavailable = errors.New("libdd_discovery unavailable: system-probe built without cgo")

// rustGetServices is the no-cgo stub for the Rust-backed getServices path.
// It lets the package compile under CGO_ENABLED=0 GOOS=linux (go vet, go
// list, static analysers) and surfaces a clear error if discovery.use_rust_library
// is ever set in a no-cgo build.
func rustGetServices(_ core.Params) (*model.ServicesResponse, error) {
	return nil, errRustLibraryUnavailable
}

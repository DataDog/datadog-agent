// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && (!bpf || !cgo)

package module

import (
	"errors"
	"os"
)

// openLogFile fails in builds without libdd_discovery, like the discovery
// module (see pkg/discovery/module/impl_rust_stub_linux.go).
func openLogFile(string, bool) (*os.File, error) {
	return nil, errors.New("libdd_discovery unavailable in this build")
}

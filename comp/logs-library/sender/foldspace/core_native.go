// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build foldspace

package foldspace

import "fmt"

// BuiltWithFoldspace is whether this binary was compiled with the foldspace tag.
const BuiltWithFoldspace = true

// NewNativeCore constructs a Core backed by libfoldspace_go.
//
// Link the native library with CGO_LDFLAGS (produced by `dda inv foldspace.build`).
// Until that library is present this constructor returns an error so tagged
// tests can skip rather than fail to link.
func NewNativeCore(_ Config) (Core, error) {
	return nil, fmt.Errorf("foldspace native core requires libfoldspace_go; set CGO_LDFLAGS to the output of `dda inv foldspace.build`")
}

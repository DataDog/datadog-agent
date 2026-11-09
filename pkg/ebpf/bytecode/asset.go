// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package bytecode

// Asset represents an eBPF asset and whether it should load the debug version
type Asset struct {
	Name  string
	Debug bool
}

// Filename returns the filename for the asset
func (c Asset) Filename() string {
	return c.FilenameWithoutExt() + ".o"
}

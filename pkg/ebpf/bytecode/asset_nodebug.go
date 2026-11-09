// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf && !bpf_debug

package bytecode

// FilenameWithoutExt returns the asset filename without an extension
func (c Asset) FilenameWithoutExt() string {
	return c.Name
}

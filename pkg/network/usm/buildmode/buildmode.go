// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package buildmode contains a definition for the different build modes supported by USM.
package buildmode

// Type represents the different options for build mode - prebuilt, runtime compilation and CO-RE.
type Type string

const (
	// Prebuilt mode
	Prebuilt Type = "prebuilt"
	// RuntimeCompiled mode
	RuntimeCompiled Type = "runtime-compilation"
	// CORE mode
	CORE Type = "CO-RE"
)

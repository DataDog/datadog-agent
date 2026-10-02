// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && !yara

package yara

// EngineName identifies the rule engine compiled into this build
const EngineName = "standin"

// DefaultCompiler returns the rule compiler of this build. Without the yara build tag, no YARA
// engine is linked in, so it returns the stand-in compiler (dry run: marker strings only).
// The libyara compiler is selected by building with the yara tag.
func DefaultCompiler() Compiler {
	return StandInCompiler
}

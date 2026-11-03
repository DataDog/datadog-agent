// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package interp

import "github.com/DataDog/datadog-agent/pkg/rshell/builtins"

// platformBuiltins returns builtins that exist only on non-Windows platforms and thus
// listed by `help` and runnable only on non-Windows platforms.
func platformBuiltins() []builtins.Command {
	return nil
}

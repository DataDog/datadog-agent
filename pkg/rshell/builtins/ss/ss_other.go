// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !darwin && !windows

package ss

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// run is the stub for platforms that do not have a native implementation.
func run(_ context.Context, callCtx *builtins.CallContext, _ options) builtins.Result {
	callCtx.Errf("ss: not supported on this platform\n")
	return builtins.Result{Code: 1}
}

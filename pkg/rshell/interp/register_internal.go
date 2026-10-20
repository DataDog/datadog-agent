// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import "github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"

func init() {
	interpoption.AllowAllCommands = func() any {
		return RunnerOption(func(r *Runner) error {
			r.allowAllCommands = true
			return nil
		})
	}
}

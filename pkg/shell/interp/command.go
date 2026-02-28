// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/shell/verifier"
)

// rejectCommand is called for any command that is not a builtin.
// Since the interpreter only supports builtins, all non-builtin commands are rejected.
func (r *Runner) rejectCommand(name string) error {
	// Check for blocked builtins first for a clear error message.
	if verifier.IsBlockedBuiltin(name) {
		return fmt.Errorf("command %q is not allowed", name)
	}
	return fmt.Errorf("command %q is not allowed", name)
}

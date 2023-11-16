// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secrets decodes secret values by invoking the configured executable command
package secrets

import (
	"io"
)

// team: agent-shared-components

// Component is the component type.
type Component interface {
	// Configure the executable command that is used for decoding secrets
	Configure(command string, arguments []string, timeout, maxSize int, groupExecPerm, removeLinebreak bool)
	// Get debug information and write it to the parameter
	GetDebugInfo(w io.Writer)
	// Whether this component is enabled, if disabled other methods will only log and error and return
	IsEnabled() bool
	// Decrypt the given handle and return the corresponding secret value
	Decrypt(data []byte, origin string) ([]byte, error)
}

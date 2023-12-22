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
	Configure(command string, arguments []string, timeout, maxSize, refreshInterval int, groupExecPerm, removeLinebreak bool)
	// Get debug information and write it to the parameter
	GetDebugInfo(w io.Writer)
	// Resolve resolves the secrets in the given yaml data by replacing secrets handles by their corresponding secret value
	Resolve(data []byte, origin string) ([]byte, error)
	// RegisterResolveCallback registers the yaml data, origin, and callback. Then it resolves the secrets in the
	// yaml data by replacing each secret handle by its corresponding secret value. The registered data can be
	// refreshed, and the callback is invoked whenever a secret value changes.
	RegisterResolveCallback(data []byte, origin string, callback ResolveCallback) error
	// Refresh will attempt to again resolve the secrets in the registered data.
	Refresh() error
}

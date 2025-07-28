// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package auditor

// Registry holds a list of offsets.
type Registry interface {
	GetOffset(identifier string) string
	GetTailingMode(identifier string) string

	// KeepAlive is used to signal that the identifier still exists and should not be removed from the registry.
	// Used for identifiers that are not guaranteed to have a tailer assigned to them.
	KeepAlive(identifier string)

	// SetTailed is used to signal that the identifier is still being tailed and should not be removed from the registry.
	SetTailed(identifier string, isTailed bool)
}

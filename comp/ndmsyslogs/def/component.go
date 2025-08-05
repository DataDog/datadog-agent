// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ndmsyslogs ... /* TODO: detailed doc comment for the component */
package ndmsyslogs

// team: /* TODO: add team name */

// Component is the component type.
type Component interface {
	// Start starts the NDM syslogs listener
	Start() error

	// Stop stops the NDM syslogs listener
	Stop()
}

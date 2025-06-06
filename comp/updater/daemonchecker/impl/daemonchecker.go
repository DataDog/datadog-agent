// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package daemoncheckerimpl implements the daemonchecker component interface
package daemoncheckerimpl

import (
	daemonchecker "github.com/DataDog/datadog-agent/comp/updater/daemonchecker/def"
)

// Requires defines the dependencies for the daemonchecker component
type Requires struct {
}

// Provides defines the output of the daemonchecker component
type Provides struct {
	Comp daemonchecker.Component
}

type checkerImpl struct{}

// NewComponent creates a new daemonchecker component
func NewComponent(_ Requires) (Provides, error) {
	return Provides{
		Comp: &checkerImpl{},
	}, nil
}

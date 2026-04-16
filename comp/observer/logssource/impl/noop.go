// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet && !docker

// Package logssourceimpl implements the logssource component.
package logssourceimpl

import (
	compdef "github.com/DataDog/datadog-agent/comp/def"
	logssource "github.com/DataDog/datadog-agent/comp/observer/logssource/def"
)

// Requires defines the dependencies for the logssource component (noop build).
type Requires struct {
	compdef.In
}

// Provides defines the output of the logssource component (noop build).
type Provides struct {
	compdef.Out
	Comp logssource.Component
}

type noopComponent struct{}

// NewComponent returns a no-op component for builds without kubelet or docker support.
func NewComponent(_ Requires) (Provides, error) {
	return Provides{Comp: &noopComponent{}}, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package startupsequencernoopimpl provides a no-op startupsequencer that always
// runs deferred work inline, for binaries that do not drive a staged startup.
package startupsequencernoopimpl

import (
	"context"

	startupsequencer "github.com/DataDog/datadog-agent/comp/core/startupsequencer/def"
)

// Provides defines the output of the no-op startupsequencer component.
type Provides struct {
	Comp startupsequencer.Component
}

type noopSequencer struct{}

// NewComponent returns a startupsequencer that runs all deferred work inline.
func NewComponent() Provides {
	return Provides{Comp: noopSequencer{}}
}

func (noopSequencer) Defer(_ startupsequencer.Stage, _ string, fn func(context.Context) error) error {
	return fn(context.Background())
}

func (noopSequencer) Begin(context.Context) {}

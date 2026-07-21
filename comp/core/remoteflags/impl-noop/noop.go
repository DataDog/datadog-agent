// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package noopimpl provides a no-op implementation of the Remote Flags component.
// It satisfies remoteflags.Component so consumers can depend on the component even
// when the feature is disabled, without fx panicking on a missing constructor.
package noopimpl

import (
	comp "github.com/DataDog/datadog-agent/comp/core/remoteflags/def"
	"github.com/DataDog/datadog-agent/pkg/remoteflags"
)

type noopComponent struct{}

// NewComponent returns a no-op Remote Flags component.
func NewComponent() comp.Component {
	return &noopComponent{}
}

// GetClient returns nil. The only caller (the RC listener wiring) is gated by
// the same `remote_flags.enabled` config and never reaches this code path when
// the component is the no-op implementation.
func (c *noopComponent) GetClient() *remoteflags.Client {
	return nil
}

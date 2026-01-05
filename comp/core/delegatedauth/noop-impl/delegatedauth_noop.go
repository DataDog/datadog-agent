// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package delegatedauthimpl provides a no-op implementation of the delegatedauth component
package delegatedauthimpl

import (
	"context"

	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
)

// Provides list the provided interfaces from the delegatedauth Component
type Provides struct {
	Comp delegatedauth.Component
}

// Requires list the required objects to initialize the noop delegatedauth Component
type Requires struct {
	Log interface{} // Accept any log component or nil
}

type delegatedAuthNoop struct{}

var _ delegatedauth.Component = (*delegatedAuthNoop)(nil)

// NewComponent returns a no-op implementation for the delegated auth component
func NewComponent(_ Requires) Provides {
	return Provides{
		Comp: &delegatedAuthNoop{},
	}
}

// Configure does nothing in the noop implementation
func (d *delegatedAuthNoop) Configure(_ delegatedauth.ConfigParams) {}

// GetAPIKey returns nil as there's no delegated auth in noop mode
func (d *delegatedAuthNoop) GetAPIKey(_ context.Context) (*string, error) {
	return nil, nil
}

// RefreshAPIKey does nothing in the noop implementation
func (d *delegatedAuthNoop) RefreshAPIKey(_ context.Context) error {
	return nil
}

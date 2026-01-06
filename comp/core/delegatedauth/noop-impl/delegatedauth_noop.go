// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package delegatedauthimpl provides a no-op implementation of the delegatedauth component
package delegatedauthimpl

import (
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
)

// Provides list the provided interfaces from the delegatedauth Component
type Provides struct {
	Comp delegatedauth.Component
}

type delegatedAuthNoop struct{}

var _ delegatedauth.Component = (*delegatedAuthNoop)(nil)

// NewComponent returns a no-op implementation for the delegated auth component
func NewComponent() Provides {
	return Provides{
		Comp: &delegatedAuthNoop{},
	}
}

// Configure does nothing in the noop implementation
func (d *delegatedAuthNoop) Configure(_ delegatedauth.ConfigParams) {}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package schemanoopimpl implements a schema component that never reports any violation. It's used by binaries that
// don't validate their configuration, so they don't link the JSON schema library and the embedded schemas.
package schemanoopimpl

import (
	schema "github.com/DataDog/datadog-agent/comp/core/schema/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// Provides defines the output of the noop schema component
type Provides struct {
	Comp schema.Component
}

// SchemaNoop is a schema component that never reports any violation. It's exported for the code loading a
// configuration outside of Fx.
type SchemaNoop struct{}

// NewComponent returns the noop schema component
func NewComponent() Provides {
	return Provides{Comp: &SchemaNoop{}}
}

// ValidateConfig never reports any violation
func (*SchemaNoop) ValidateConfig(model.Reader) ([]*model.Violation, error) {
	return nil, nil
}

// ValidateYAML never reports any violation
func (*SchemaNoop) ValidateYAML([]byte, model.SchemaID) ([]*model.Violation, error) {
	return nil, nil
}

// GetViolations never reports any violation
func (*SchemaNoop) GetViolations(model.SchemaID) []*model.Violation {
	return nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package schema provides a component validating the Agent configurations against their schemas
package schema

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// team: fleet-automation

// Component validates configurations against the schemas embedded in the Agent.
//
// Validating a configuration links the JSON schema library and the embedded schemas into the binary. Binaries that
// don't validate their configuration use the noop implementation to stay small.
type Component interface {
	// ValidateConfig validates the settings of config against its schema (see model.Reader.GetSchemaID), records
	// the violations for GetViolations and returns them.
	//
	// It's meant to be called by the configuration while it loads: once the configuration file and the environment
	// variables are loaded, but before secrets are resolved and before the Agent changes any setting at runtime.
	// Secrets are therefore still ENC[] placeholders, which are not validated.
	ValidateConfig(config model.Reader) ([]*model.Violation, error)

	// GetViolations returns the violations recorded by the last ValidateConfig for the schemaID.
	GetViolations(schemaID model.SchemaID) []*model.Violation

	// ValidateYAML validates raw YAML contents against the schema identified by schemaID.
	ValidateYAML(data []byte, schemaID model.SchemaID) ([]*model.Violation, error)
}

// The config packages receive the component through the model.SchemaValidator interface
var _ model.SchemaValidator = (Component)(nil)

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package status ... /* TODO: detailed doc comment for the component */
package status

import (
	"io"

	"go.uber.org/fx"
)

// team: agent-shared-components

// Component is the component type.
type Component interface {
	// /* TODO: detailed doc comment for the component */
	GetStatus(format string, verbose bool) ([]byte, error)
	GetStatusSection(section, format string, verbose bool) ([]byte, error)
}

type StatusProvider interface {
	Name() string
	JSON(stats map[string]interface{})
	Text(buffer io.Writer) error
}

type Provider struct {
	fx.Out

	Provider StatusProvider `group:"status"`
}

// NewProvider returns a new Provider to be called when a flare is created
func NewProvider(provider StatusProvider) Provider {
	return Provider{
		Provider: provider,
	}
}

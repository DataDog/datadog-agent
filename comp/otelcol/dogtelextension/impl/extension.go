// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dogtelextensionimpl provides the implementation for the Dogtel extension.
package dogtelextensionimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/otelcol/dogtelextension/def"
	"go.opentelemetry.io/collector/component"
)

// dogtelExtension implements the dogtelextension.Component interface
type dogtelExtension struct{}

var _ dogtelextension.Component = (*dogtelExtension)(nil)

// NewComponent creates a new dogtelextension component
func NewComponent() dogtelextension.Component {
	return &dogtelExtension{}
}

// Start implements extension.Extension
func (e *dogtelExtension) Start(_ context.Context, _ component.Host) error {
	return nil
}

// Shutdown implements extension.Extension
func (e *dogtelExtension) Shutdown(_ context.Context) error {
	return nil
}

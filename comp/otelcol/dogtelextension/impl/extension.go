// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogtelextensionimpl

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"github.com/DataDog/datadog-agent/comp/otelcol/dogtelextension/def"
)

// dogtelExtension implements the dogtelextension.Component interface
type dogtelExtension struct{}

var _ dogtelextension.Component = (*dogtelExtension)(nil)

// NewComponent creates a new dogtelextension component
func NewComponent() dogtelextension.Component {
	return &dogtelExtension{}
}

// Start implements extension.Extension
func (e *dogtelExtension) Start(ctx context.Context, host component.Host) error {
	return nil
}

// Shutdown implements extension.Extension
func (e *dogtelExtension) Shutdown(ctx context.Context) error {
	return nil
}

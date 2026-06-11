// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package runner implements a component to generate metadata payload at the right interval.
package runner

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: agent-configuration

// Component is the component type.
type Component interface{}

// MetadataProvider is the callback type for metadata providers registered with the runner.
type MetadataProvider func(context.Context) time.Duration

// Provider registers a MetadataProvider callback with the runner's fx group.
type Provider struct {
	fx.Out

	Callback MetadataProvider `group:"metadata_provider"`
}

// NewProvider registers a new metadata provider.
func NewProvider(callback MetadataProvider) Provider {
	return Provider{Callback: callback}
}

// GetAndFilterProviders filters nil providers from the fx group.
// This wraps fxutil.GetAndFilterGroup so that impl packages can use it
// without importing fxutil directly.
var GetAndFilterProviders = fxutil.GetAndFilterGroup[[]MetadataProvider]

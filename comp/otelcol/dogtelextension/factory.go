// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogtelextension

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

var (
	// Type is the type string for this extension
	Type = component.MustNewType("dogtel")
)

const (
	stability = component.StabilityLevelAlpha
)

// factory stores FX-injected components for extension creation
type factory struct {
	config     coreconfig.Component
	log        log.Component
	serializer serializer.MetricSerializer
	hostname   hostnameinterface.Component
	tagger     tagger.Component
	ipc        ipc.Component
	telemetry  telemetry.Component
}

// NewFactory creates a basic factory (for standalone OTel collector builds)
// This factory will return an error since the extension requires agent components
func NewFactory() extension.Factory {
	return extension.NewFactory(
		Type,
		createDefaultConfig,
		func(ctx context.Context, params extension.Settings, cfg component.Config) (extension.Extension, error) {
			return nil, fmt.Errorf("dogtelextension requires agent components and is not OCB-compliant")
		},
		stability,
	)
}

// NewFactoryForAgent creates factory with FX component injection (for otel-agent)
func NewFactoryForAgent(
	config coreconfig.Component,
	log log.Component,
	serializer serializer.MetricSerializer,
	hostname hostnameinterface.Component,
	tagger tagger.Component,
	ipc ipc.Component,
	telemetry telemetry.Component,
) extension.Factory {
	f := &factory{
		config:     config,
		log:        log,
		serializer: serializer,
		hostname:   hostname,
		tagger:     tagger,
		ipc:        ipc,
		telemetry:  telemetry,
	}

	return extension.NewFactory(
		Type,
		createDefaultConfig,
		func(ctx context.Context, params extension.Settings, cfg component.Config) (extension.Extension, error) {
			return newExtension(ctx, params, cfg.(*Config), f)
		},
		stability,
	)
}

func newExtension(
	_ context.Context,
	_ extension.Settings,
	cfg *Config,
	f *factory,
) (extension.Extension, error) {
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create extension with all dependencies
	ext := &dogtelExtension{
		config:     cfg,
		log:        f.log,
		serializer: f.serializer,
		hostname:   f.hostname,
		tagger:     f.tagger,
		ipc:        f.ipc,
		telemetry:  f.telemetry,
		coreConfig: f.config,
	}

	return ext, nil
}

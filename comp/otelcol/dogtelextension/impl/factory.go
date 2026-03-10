// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogtelextensionimpl

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	dogtelextension "github.com/DataDog/datadog-agent/comp/otelcol/dogtelextension/def"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

var (
	// Type is the type string for this extension
	Type = component.MustNewType("dogtel")
)

const (
	stability = component.StabilityLevelAlpha
)

// componentHolder stores FX-injected components for extension creation
type componentHolder struct {
	config       coreconfig.Component
	log          log.Component
	serializer   serializer.MetricSerializer
	hostname     hostnameinterface.Component
	workloadmeta workloadmeta.Component
	tagger       tagger.Component
	ipc          ipc.Component
	telemetry    telemetry.Component
	secrets      secrets.Component
}

// NewFactory creates a basic factory (for standalone OTel collector builds)
// This factory will return an error since the extension requires agent components
func NewFactory() extension.Factory {
	return extension.NewFactory(
		Type,
		createDefaultConfig,
		func(_ context.Context, _ extension.Settings, _ component.Config) (extension.Extension, error) {
			return nil, errors.New("dogtelextension requires agent components, use NewFactoryForAgent()")
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
	workloadmeta workloadmeta.Component,
	tagger tagger.Component,
	ipc ipc.Component,
	telemetry telemetry.Component,
	secrets secrets.Component,
) extension.Factory {
	components := &componentHolder{
		config:       config,
		log:          log,
		serializer:   serializer,
		hostname:     hostname,
		workloadmeta: workloadmeta,
		tagger:       tagger,
		ipc:          ipc,
		telemetry:    telemetry,
		secrets:      secrets,
	}

	return extension.NewFactory(
		Type,
		createDefaultConfig,
		func(_ context.Context, params extension.Settings, cfg component.Config) (extension.Extension, error) {
			return newExtension(cfg.(*Config), components, params.BuildInfo)
		},
		stability,
	)
}

// Requires defines the dependencies needed to create a dogtelExtension via FX.
type Requires struct {
	Config       coreconfig.Component
	Log          log.Component
	Serializer   serializer.MetricSerializer
	Hostname     hostnameinterface.Component
	Workloadmeta workloadmeta.Component
	Tagger       tagger.Component
	IPC          ipc.Component
	Telemetry    telemetry.Component
	Secrets      secrets.Component
}

// NewExtension creates a new dogtelextension instance for use with FX.
func NewExtension(reqs Requires) (dogtelextension.Component, error) {
	cfg := createDefaultConfig().(*Config)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	return &dogtelExtension{
		config:       cfg,
		log:          reqs.Log,
		coreConfig:   reqs.Config,
		serializer:   reqs.Serializer,
		hostname:     reqs.Hostname,
		workloadmeta: reqs.Workloadmeta,
		tagger:       reqs.Tagger,
		ipc:          reqs.IPC,
		telemetry:    reqs.Telemetry,
		secrets:      reqs.Secrets,
		buildInfo:    component.BuildInfo{},
	}, nil
}

func createDefaultConfig() component.Config {
	return &Config{
		EnableMetadataCollection: true,
		MetadataInterval:         300, // 5 minutes
		EnableTaggerServer:       false,
		TaggerServerPort:         0, // Auto-assign
		TaggerServerAddr:         "localhost",
		TaggerMaxMessageSize:     4 * 1024 * 1024, // 4MB
		TaggerMaxConcurrentSync:  5,
		StandaloneMode:           false, // Default: connected mode
	}
}

func newExtension(
	cfg *Config,
	components *componentHolder,
	buildInfo component.BuildInfo,
) (dogtelextension.Component, error) {
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create extension with all dependencies
	ext := &dogtelExtension{
		config:       cfg,
		log:          components.log,
		serializer:   components.serializer,
		hostname:     components.hostname,
		workloadmeta: components.workloadmeta,
		tagger:       components.tagger,
		ipc:          components.ipc,
		telemetry:    components.telemetry,
		secrets:      components.secrets,
		coreConfig:   components.config,
		buildInfo:    buildInfo,
	}

	return ext, nil
}

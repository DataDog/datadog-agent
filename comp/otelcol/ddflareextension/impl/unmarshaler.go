// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"fmt"

	"golang.org/x/exp/maps"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/service"
	"go.opentelemetry.io/collector/service/telemetry"
)

type configSettings struct {
	Receivers  *configs[receiver.Factory]  `mapstructure:"receivers"`
	Processors *configs[processor.Factory] `mapstructure:"processors"`
	Exporters  *configs[exporter.Factory]  `mapstructure:"exporters"`
	Connectors *configs[connector.Factory] `mapstructure:"connectors"`
	Extensions *configs[extension.Factory] `mapstructure:"extensions"`
	Service    service.Config              `mapstructure:"service"`
}

// unmarshal the configSettings from a confmap.Conf.
// After the config is unmarshalled, `Validate()` must be called to validate.
func unmarshal(v *confmap.Conf, factories otelcol.Factories) (*configSettings, error) {

	telFactory := telemetry.NewFactory()
	defaultTelConfig := *telFactory.CreateDefaultConfig().(*telemetry.Config)

	// Unmarshal top level sections and validate.
	cfg := &configSettings{
		Receivers:  newConfigs(factories.Receivers),
		Processors: newConfigs(factories.Processors),
		Exporters:  newConfigs(factories.Exporters),
		Connectors: newConfigs(factories.Connectors),
		Extensions: newConfigs(factories.Extensions),
		// TODO: Add a component.ServiceFactory to allow this to be defined by the Service.
		Service: service.Config{
			Telemetry: defaultTelConfig,
		},
	}

	return cfg, v.Unmarshal(&cfg)
}

type configs[F component.Factory] struct {
	cfgs map[component.ID]component.Config

	factories map[component.Type]F
}

func newConfigs[F component.Factory](factories map[component.Type]F) *configs[F] {
	return &configs[F]{factories: factories}
}

func (c *configs[F]) Configs() map[component.ID]component.Config {
	return c.cfgs
}

func (c *configs[F]) Unmarshal(conf *confmap.Conf) error {
	rawCfgs := make(map[component.ID]map[string]any)
	if err := conf.Unmarshal(&rawCfgs); err != nil {
		return err
	}

	// Prepare resulting map.
	c.cfgs = make(map[component.ID]component.Config)
	// Iterate over raw configs and create a config for each.
	for id := range rawCfgs {
		// Find factory based on component kind and type that we read from config source.
		factory, ok := c.factories[id.Type()]
		if !ok {
			return errorUnknownType(id, maps.Keys(c.factories))
		}

		// Get the configuration from the confmap.Conf to preserve internal representation.
		sub, err := conf.Sub(id.String())
		if err != nil {
			return errorUnmarshalError(id, err)
		}

		// Create the default config for this component.
		cfg := factory.CreateDefaultConfig()

		// Now that the default config struct is created we can Unmarshal into it,
		// and it will apply user-defined config on top of the default.
		if err := sub.Unmarshal(&cfg); err != nil {
			return errorUnmarshalError(id, err)
		}

		c.cfgs[id] = cfg
	}

	return nil
}

func errorUnknownType(id component.ID, factories []component.Type) error {
	return fmt.Errorf("unknown type: %q for id: %q (valid values: %v)", id.Type(), id, factories)
}

func errorUnmarshalError(id component.ID, err error) error {
	return fmt.Errorf("error reading configuration for %q: %w", id, err)
}

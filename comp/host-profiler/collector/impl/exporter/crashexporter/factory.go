// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package crashexporter

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/xexporter"
	"go.opentelemetry.io/collector/pdata/pprofile"
)

const typeStr = "crash"

// NewFactory creates the crash exporter factory.
func NewFactory() exporter.Factory {
	return xexporter.NewFactory(
		component.MustNewType(typeStr),
		defaultConfig,
		xexporter.WithProfiles(createProfilesExporter, component.StabilityLevelAlpha),
	)
}

func defaultConfig() component.Config {
	return &Config{}
}

func createProfilesExporter(
	_ context.Context,
	_ exporter.Settings,
	cfg component.Config,
) (xexporter.Profiles, error) {
	c := cfg.(*Config)
	return &profilesExporter{crashExporter: newExporter(c)}, nil
}

// profilesExporter wraps crashExporter to implement xexporter.Profiles.
type profilesExporter struct {
	*crashExporter
}

func (e *profilesExporter) Start(_ context.Context, _ component.Host) error  { return nil }
func (e *profilesExporter) Shutdown(_ context.Context) error                   { return nil }
func (e *profilesExporter) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}
func (e *profilesExporter) ConsumeProfiles(ctx context.Context, profiles pprofile.Profiles) error {
	return e.crashExporter.ConsumeProfiles(ctx, profiles)
}

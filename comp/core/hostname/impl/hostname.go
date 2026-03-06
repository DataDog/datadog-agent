// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostnameimpl implements the hostname component.
package hostnameimpl

import (
	"context"

	compdef "github.com/DataDog/datadog-agent/comp/def"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamedef "github.com/DataDog/datadog-agent/comp/core/hostname/def"
	telemetrycomp "github.com/DataDog/datadog-agent/comp/core/telemetry"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// Requires defines the dependencies for the hostname component.
type Requires struct {
	compdef.In

	Config    config.Component
	Lc        compdef.Lifecycle
	Telemetry telemetrycomp.Component
}

// Provides defines the output of the hostname component.
type Provides struct {
	compdef.Out

	Comp hostnamedef.Component
}

type service struct {
	config config.Component
	drift  *driftService
}

var _ hostnamedef.Component = (*service)(nil)

// NewComponent creates a new hostname component.
func NewComponent(reqs Requires) Provides {
	svc := &service{
		config: reqs.Config,
		drift:  newDriftService(reqs.Config, reqs.Telemetry),
	}
	reqs.Lc.Append(compdef.Hook{
		OnStart: svc.startDriftMonitoring,
		OnStop:  svc.stopDriftMonitoring,
	})
	return Provides{Comp: svc}
}

// Get returns the hostname.
func (s *service) Get(ctx context.Context) (string, error) {
	data, err := s.GetWithProvider(ctx)
	return data.Hostname, err
}

// GetWithProvider returns the hostname and the provider that was used to retrieve it.
func (s *service) GetWithProvider(ctx context.Context) (hostnamedef.Data, error) {
	return getHostname(ctx, s.config, "hostname", false)
}

// GetSafe returns the hostname, or 'unknown host' if anything goes wrong.
func (s *service) GetSafe(ctx context.Context) string {
	name, err := s.Get(ctx)
	if err != nil {
		return "unknown host"
	}
	return name
}

func (s *service) startDriftMonitoring(_ context.Context) error {
	hostnameData, err := getHostname(context.Background(), s.config, "hostname", false)
	if err != nil {
		// Hostname not resolved yet — drift monitoring will be skipped. This is acceptable;
		// the hostname will still be resolved on the first caller of Get/GetWithProvider.
		return nil
	}
	s.drift.start(hostnameData)
	return nil
}

func (s *service) stopDriftMonitoring(_ context.Context) error {
	s.drift.stop()
	return nil
}

// GetWithProviderFromConfig resolves the hostname using the provided config reader.
// This function is intended for use outside the fx component graph.
// Prefer the Component interface when possible.
func GetWithProviderFromConfig(ctx context.Context, cfg pkgconfigmodel.Reader) (hostnamedef.Data, error) {
	return getHostname(ctx, cfg, "hostname", false)
}

// GetWithLegacyResolutionProviderFromConfig resolves the hostname for the EC2 IMDSv2 transition.
// This function is intended for use outside the fx component graph.
func GetWithLegacyResolutionProviderFromConfig(ctx context.Context, cfg pkgconfigmodel.Reader) (hostnamedef.Data, error) {
	// If the user has set ec2_prefer_imdsv2 then IMDSv2 is used by default — legacy resolution is not needed.
	// If ec2_imdsv2_transition_payload_enabled is not set the agent does not need the legacy hostname.
	if cfg.GetBool("ec2_prefer_imdsv2") || !cfg.GetBool("ec2_imdsv2_transition_payload_enabled") {
		return hostnamedef.Data{}, nil
	}
	return getHostname(ctx, cfg, "legacy_resolution_hostname", true)
}

// GetFromConfig resolves just the hostname string using the provided config reader.
// This function is intended for use outside the fx component graph.
func GetFromConfig(ctx context.Context, cfg pkgconfigmodel.Reader) (string, error) {
	data, err := GetWithProviderFromConfig(ctx, cfg)
	return data.Hostname, err
}

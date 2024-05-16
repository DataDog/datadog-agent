// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelflareimpl implements the OTel flare component
package otelflareimpl

import (
	flaredef "github.com/DataDog/datadog-agent/comp/core/flare/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	otelflare "github.com/DataDog/datadog-agent/comp/otelcol/otelflare/def"
)

// Provides is the outputs of the component constructor
type Provides struct {
	compdef.Out
	Comp          otelflare.Component
	FlareProvider flaredef.Provider
}

// NewComponent returns a new flare provider
func NewComponent() Provides {
	impl := &otelflareImpl{}
	return Provides{
		Comp:          impl,
		FlareProvider: flaredef.NewProvider(impl.fillFlare),
	}
}

type otelflareImpl struct {
	Enabled bool
}

// SetEnabled sets this component to be enabled
func (i *otelflareImpl) SetEnabled() {
	i.Enabled = true
}

func (i *otelflareImpl) fillFlare(fb flaredef.FlareBuilder) error {
	if i.Enabled {
		// TODO: placeholder for now, until OTel extension exists to provide data
		fb.AddFile("otel-agent.log", []byte("otel-agent flare")) //nolint:errcheck
	}
	return nil
}

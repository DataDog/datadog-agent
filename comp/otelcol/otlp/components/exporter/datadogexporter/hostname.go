// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package datadogexporter

import (
	"context"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
)

// hostnameService adapts a source.Provider into a hostnameinterface.Component,
// which the logs agent exporter's orchestrator path uses to stamp the host name
// onto manifest payloads. It mirrors the datadogexporter's internal hostname
// service in opentelemetry-collector-contrib, which is not importable here.
type hostnameService struct {
	provider source.Provider
}

var _ hostnameinterface.Component = (*hostnameService)(nil)

// newHostnameService builds a hostnameinterface.Component backed by the given source provider.
func newHostnameService(provider source.Provider) hostnameinterface.Component {
	return &hostnameService{provider: provider}
}

// Get returns the hostname resolved from the source provider, or an empty string
// when the source is not a hostname (e.g. a cloud resource identifier).
func (hs *hostnameService) Get(ctx context.Context) (string, error) {
	src, err := hs.provider.Source(ctx)
	if err != nil {
		return "", err
	}
	if src.Kind == source.HostnameKind {
		return src.Identifier, nil
	}
	return "", nil
}

// GetSafe is Get(), but returns "unknown host" if anything goes wrong.
func (hs *hostnameService) GetSafe(ctx context.Context) string {
	name, err := hs.Get(ctx)
	if err != nil {
		return "unknown host"
	}
	return name
}

// GetWithProvider returns the hostname alongside the (empty) provider name.
func (hs *hostnameService) GetWithProvider(ctx context.Context) (hostnameinterface.Data, error) {
	name, err := hs.Get(ctx)
	if err != nil {
		return hostnameinterface.Data{}, err
	}
	return hostnameinterface.Data{Hostname: name}, nil
}

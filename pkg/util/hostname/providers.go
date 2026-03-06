// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

// Package hostname provides thin shim functions that delegate to comp/core/hostname/impl.
// Direct use of pkg/util/hostname is deprecated; prefer the comp/core/hostname component
// or comp/core/hostname/impl standalone functions.
package hostname

import (
	"context"

	hostnameimpl "github.com/DataDog/datadog-agent/comp/core/hostname/impl"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// GetWithProvider returns the hostname for the Agent and the provider that was used to retrieve it.
func GetWithProvider(ctx context.Context) (Data, error) {
	return hostnameimpl.GetWithProviderFromConfig(ctx, pkgconfigsetup.Datadog())
}

// GetWithLegacyResolutionProvider returns the hostname for the Agent and the provider that was
// used to retrieve it, without using IMDSv2 and MDI (for EC2 IMDSv2 transition).
func GetWithLegacyResolutionProvider(ctx context.Context) (Data, error) {
	return hostnameimpl.GetWithLegacyResolutionProviderFromConfig(ctx, pkgconfigsetup.Datadog())
}

// Get returns the host name for the agent.
func Get(ctx context.Context) (string, error) {
	data, err := GetWithProvider(ctx)
	return data.Hostname, err
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package common provides the common config and provider models for delegated auth
package common

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// AuthConfig provides cloud provider based authentication configuration.
type AuthConfig struct {
	OrgUUID      string
	Provider     string
	ProviderAuth Provider
}

// Provider is an interface for getting a delegated token utilizing different methods.
type Provider interface {
	GetAPIKey(cfg pkgconfigmodel.Reader, config *AuthConfig) (*string, error)
}

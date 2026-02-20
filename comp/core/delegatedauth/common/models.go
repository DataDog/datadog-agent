// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package common provides the common config and provider models for delegated auth
package common

import (
	"context"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// AuthConfig provides cloud provider based authentication configuration.
type AuthConfig struct {
	OrgUUID string
}

// ProviderConfig is an interface for provider-specific configuration.
// Each cloud provider implements its own config struct with the fields it needs.
type ProviderConfig interface {
	// ProviderName returns the name of the provider (e.g., "aws").
	ProviderName() string
}

// Provider is an interface for generating cloud-specific authentication proofs.
// Each provider implements how to generate a proof from their cloud platform (e.g., AWS SigV4 signed request).
// The proof is then exchanged for a Datadog API key using api.GetAPIKey().
type Provider interface {
	// GenerateAuthProof generates a cloud-specific authentication proof string.
	// This proof will be passed to Datadog's intake-key API to exchange for an API key.
	// The context allows for cancellation of the proof generation.
	GenerateAuthProof(ctx context.Context, cfg pkgconfigmodel.Reader, config *AuthConfig) (string, error)
}

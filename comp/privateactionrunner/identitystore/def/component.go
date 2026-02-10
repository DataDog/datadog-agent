// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package identitystore provides an interface for storing and retrieving PAR identity
package identitystore

import (
	"context"
)

// team: action-platform

// Component is the component type for identity storage
type Component interface {
	// GetIdentity retrieves the persisted PAR identity
	// Returns nil if no identity exists
	GetIdentity(ctx context.Context) (*Identity, error)

	// PersistIdentity saves the PAR identity
	PersistIdentity(ctx context.Context, identity *Identity) error

	// DeleteIdentity removes the persisted identity (for testing/cleanup)
	DeleteIdentity(ctx context.Context) error
}

// Identity represents a persisted PAR identity
type Identity struct {
	// PrivateKey is the base64-encoded JWK private key
	PrivateKey string `json:"private_key"`
	// URN is the unique runner name (urn:dd:apps:on-prem-runner:{region}:{orgId}:{runnerId})
	URN string `json:"urn"`
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package common

import (
	"context"
	"encoding/base64"
	"errors"
	"regexp"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// WorkloadTokenRequest asks ETS for explicit grants. Key binding is optional at
// the protocol level; the par_enrollment grant requires a public-key thumbprint.
type WorkloadTokenRequest struct {
	Grants        []string `json:"grants"`
	JWKThumbprint string   `json:"jwk_thumbprint,omitempty"`
}

var workloadGrantName = regexp.MustCompile(`^[a-z0-9_]+$`)

// Validate rejects ambiguous signed inputs before requesting a cloud proof.
// ETS owns grant authorization and grant-specific requirements.
func (r WorkloadTokenRequest) Validate() error {
	if len(r.Grants) == 0 || len(r.Grants) > 16 {
		return errors.New("expected 1 to 16 workload grants")
	}
	seen := make(map[string]bool, len(r.Grants))
	for _, grant := range r.Grants {
		if seen[grant] || !workloadGrantName.MatchString(grant) {
			return errors.New("invalid or duplicate workload grant")
		}
		seen[grant] = true
	}
	if r.JWKThumbprint != "" {
		key, err := base64.RawURLEncoding.DecodeString(r.JWKThumbprint)
		if err != nil || len(key) != 32 || base64.RawURLEncoding.EncodeToString(key) != r.JWKThumbprint {
			return errors.New("invalid workload key thumbprint")
		}
	}
	return nil
}

// WorkloadProvider explicitly supports binding a proof to the requested grants
// and key. Implementing Provider for intake alone does not imply this capability.
type WorkloadProvider interface {
	GenerateWorkloadAuthProof(context.Context, pkgconfigmodel.Reader, *AuthConfig, WorkloadTokenRequest) (string, error)
}

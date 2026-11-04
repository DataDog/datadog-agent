// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package oidc

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

var _ common.WorkloadProvider = (*Auth)(nil)

// GenerateWorkloadAuthProof obtains a fresh OIDC bearer proof for the workload
// exchange. ETS authorizes the requested grants. The provider JWT does not sign
// a caller-supplied public key, so this adapter cannot request key binding.
func (a *Auth) GenerateWorkloadAuthProof(ctx context.Context, cfg pkgconfigmodel.Reader, config *common.AuthConfig, request common.WorkloadTokenRequest) (string, error) {
	if err := request.Validate(); err != nil {
		return "", err
	}
	if request.JWKThumbprint != "" {
		return "", errors.New("OIDC bearer proofs cannot bind a workload public key")
	}
	return a.GenerateAuthProof(ctx, cfg, config)
}

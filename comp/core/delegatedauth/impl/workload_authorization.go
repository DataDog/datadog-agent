// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package delegatedauthimpl

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/api"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
)

// GetWorkloadAuthorization explicitly selects an org/site instance. PAR should
// use the primary "api_key" instance, never an arbitrary dual-shipping instance.
func (d *delegatedAuthComponent) GetWorkloadAuthorization(ctx context.Context, apiKeyConfigKey, jwkThumbprint string) (*common.WorkloadAuthorization, error) {
	d.mu.RLock()
	instance := d.instances[apiKeyConfigKey]
	if instance == nil || instance.provider == nil || instance.authConfig == nil || d.config == nil {
		d.mu.RUnlock()
		return nil, common.ErrWorkloadAuthorizationUnavailable
	}
	provider, config, target := instance.provider, d.config, instance.targetSite
	authConfig := *instance.authConfig
	d.mu.RUnlock()

	proof, err := provider.GenerateAuthProof(ctx, config, &authConfig)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("failed to generate workload authorization proof")
	}
	result, err := api.GetWorkloadAuthorization(ctx, config, proof, target, authConfig.OrgUUID, jwkThumbprint)
	if err != nil {
		return nil, err
	}
	// Configuration may have been replaced while the exchange was in flight.
	// Do not return an assertion for the old organization/site in that case.
	d.mu.RLock()
	unchanged := d.instances[apiKeyConfigKey] == instance
	d.mu.RUnlock()
	if !unchanged {
		return nil, common.ErrWorkloadAuthorizationUnavailable
	}
	return result, nil
}

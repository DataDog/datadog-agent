// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package oidc provides the custom OIDC delegated auth provider.
package oidc

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// OIDCAuth generates custom OIDC proofs from a Datadog-scoped workload token file.
type OIDCAuth struct {
	tokenFile string
	now       func() time.Time
}

var _ common.Provider = (*OIDCAuth)(nil)

// NewOIDCAuth validates the configuration and returns a provider for it.
func NewOIDCAuth(config *cloudauthconfig.OIDCProviderConfig) (*OIDCAuth, error) {
	if config == nil || config.TokenFile == "" {
		return nil, errors.New("delegated_auth.oidc.token_file is required for the oidc provider")
	}
	if !filepath.IsAbs(config.TokenFile) {
		return nil, errors.New("delegated_auth.oidc.token_file must be an absolute path")
	}
	return &OIDCAuth{tokenFile: config.TokenFile, now: time.Now}, nil
}

// GenerateAuthProof re-reads the token file and refuses it, without any request, unless it is scoped to the org.
func (a *OIDCAuth) GenerateAuthProof(ctx context.Context, _ pkgconfigmodel.Reader, config *common.AuthConfig) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if config == nil {
		return "", errors.New("missing delegated auth configuration")
	}
	return proof(a.tokenFile, config.OrgUUID, a.now())
}

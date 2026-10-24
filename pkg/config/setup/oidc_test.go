// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package setup

import (
	"context"
	"testing"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	"github.com/stretchr/testify/require"
)

func TestOIDCConfigAndEnvironment(t *testing.T) {
	t.Setenv("DD_DELEGATED_AUTH_OIDC_SOURCE", "azure")
	t.Setenv("DD_DELEGATED_AUTH_OIDC_AUDIENCE", "api://datadog")
	c := confFromYAML(t, `
delegated_auth:
  provider: oidc
  org_uuid: primary-org
  oidc:
    source: gcp
    client_id: managed-identity-client
additional_endpoints:
  https://app.datad0g.com:
    - DELA[other-org, oidc]
`)
	seen := map[string]bool{}
	comp := &delegatedauthmock.Mock{AddInstanceFunc: func(_ context.Context, p delegatedauth.InstanceParams) error {
		cfg, ok := p.ProviderConfig.(*cloudauthconfig.OIDCProviderConfig)
		require.True(t, ok)
		require.Equal(t, "azure", cfg.Source)
		require.Equal(t, "api://datadog", cfg.Audience)
		require.Equal(t, "managed-identity-client", cfg.ClientID)
		require.Equal(t, "request", cfg.OrgUUIDSource)
		seen[p.OrgUUID] = true
		return nil
	}}
	require.NoError(t, configureDelegatedAuth(context.Background(), c, comp))
	require.Equal(t, map[string]bool{"primary-org": true, "other-org": true}, seen)
}

func TestUnknownProviderDoesNotFallBackToAWS(t *testing.T) {
	c := confFromYAML(t, "delegated_auth:\n  provider: typo\n  org_uuid: org\n")
	comp := &delegatedauthmock.Mock{AddInstanceFunc: func(context.Context, delegatedauth.InstanceParams) error {
		t.Fatal("must not auto-detect an unknown explicit provider")
		return nil
	}}
	require.ErrorContains(t, configureDelegatedAuth(context.Background(), c, comp), "unsupported delegated_auth.provider")
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcx509clientimpl

import (
	"context"
	"os"
	"testing"
	"time"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	delegatedauthimpl "github.com/DataDog/datadog-agent/comp/core/delegatedauth/impl"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	"github.com/stretchr/testify/require"
)

// TestAWSRCSecretsPOCSmoke is an opt-in manual staging check. It uses real AWS
// credentials, intake bootstrap and ETS exchange, but substitutes a local peer
// for the tunnel/backend. It does not prove RC verification or live delivery.
// Run in a dedicated process so the intake refresh lifecycle ends on exit.
func TestAWSRCSecretsPOCSmoke(t *testing.T) {
	org := os.Getenv("RC_SECRETS_POC_ORG_UUID")
	if org == "" {
		t.Skip("manual AWS POC; see doc/how-to/rc-secrets-workload-prototype.md")
	}
	require.NotEmpty(t, os.Getenv("AWS_ACCESS_KEY_ID"))
	require.NotEmpty(t, os.Getenv("AWS_SECRET_ACCESS_KEY"))
	require.NotEmpty(t, os.Getenv("AWS_SESSION_TOKEN"))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var config rcx509client.ClientConfig
	deps, lifecycle := newDependencies(t, map[string]interface{}{
		"site":         "datad0g.com",
		enabledSetting: true, secretsEnabledSetting: true,
		secretProductsSetting: []string{"rc-secrets-poc"},
	}, func(c rcx509client.ClientConfig) (rcx509client.Client, error) {
		config = c
		return newFakeClient(), nil
	})
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}
	auth := delegatedauthimpl.NewComponent().Comp
	require.NoError(t, auth.AddInstance(ctx, delegatedauth.InstanceParams{
		Config: deps.Config, OrgUUID: org, APIKeyConfigKey: "api_key",
		TargetSite: "datad0g.com", RefreshInterval: 60,
		ProviderConfig: &cloudauthconfig.AWSProviderConfig{Region: region},
	}))
	// Show the exchange client's sanitized error before the preparation handler
	// deliberately reduces identity failures to a protocol status.
	_, exchangeErr := auth.GetWorkloadToken(ctx, "api_key", common.WorkloadTokenRequest{Grants: []string{"rc_secrets"}})
	require.NoError(t, exchangeErr)
	deps.DelegatedAuth = auth
	component := New(deps).Comp
	require.NotNil(t, config.PrepareSecretDelivery)
	require.NoError(t, lifecycle.Start(ctx))
	t.Cleanup(func() { require.NoError(t, lifecycle.Stop(context.Background())) })
	response := prepareTestSecret(t, config.PrepareSecretDelivery, "rc-secrets-poc", "aws-smoke")
	if response.Token == "" {
		t.Fatal("preparation did not return a workload token")
	}
	ciphertext := sealTestSecret(t, response)
	plaintext, err := component.OpenSecret("rc-secrets-poc", "aws-smoke", ciphertext)
	require.NoError(t, err)
	require.Equal(t, "example secret", string(plaintext))
	clear(plaintext)
	_, err = component.OpenSecret("rc-secrets-poc", "aws-smoke", ciphertext)
	require.Error(t, err)
	t.Log("RC_AWS_POC_VERIFIED=intake-bootstrap,workload-exchange,preparation,local-decryption,single-use")
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package enrollment

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	partypes "github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/par"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/jsonapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"
)

func TestClusterWIFEnrollmentSharesIdentity(t *testing.T) {
	flavor.SetTestFlavor(t, flavor.ClusterAgent)
	// No live cluster discovery: use the existing hostname fallback for the name.
	env.SetFeatures(t)
	t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
	received := make(chan []byte, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/unstable/on_prem_runners/workload_token", r.URL.Path)
		assert.Equal(t, "Bearer workload-token", r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("DD-API-KEY"))
		assert.Empty(t, r.Header.Get("DD-APPLICATION-KEY"))
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		received <- body
		fmt.Fprint(w, `{"data":{"type":"createRunnerResponse","id":"runner-cluster","attributes":{"org_id":42,"runner_id":"runner-cluster"}}}`)
	}))
	defer server.Close()
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"dd_url": server.URL, "skip_ssl_validation": true, "api_key": "unused-intake-key",
		"delegated_auth.org_uuid": "test-org", "site": "DatadogHQ.EU.",
	})
	var signedRequest common.WorkloadTokenRequest
	auth := &delegatedauthmock.Mock{GetWorkloadTokenFunc: func(_ context.Context, key string, request common.WorkloadTokenRequest) (string, error) {
		require.Equal(t, "api_key", key)
		require.Equal(t, []string{"par_enrollment"}, request.Grants)
		signedRequest = request
		return "workload-token", nil
	}}
	result, err := EnrollWithWorkloadToken(context.Background(), cfg, auth, &AgentIdentifier{Hostname: "replica-a", OrchClusterID: "cluster-id"})
	require.NoError(t, err)
	var request partypes.CreateRunnerRequest
	require.NoError(t, jsonapi.Unmarshal(<-received, &request))
	require.Equal(t, flavor.ClusterAgent, request.AgentFlavor)
	require.Equal(t, "cluster-id", request.OrchClusterID)
	require.Empty(t, request.AgentHostname, "a shared runner must not be bound to one replica's host")
	require.Contains(t, request.RunnerName, "replica-a-")
	block, _ := pem.Decode([]byte(request.PublicKeyPEM))
	require.NotNil(t, block)
	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	require.NoError(t, err)
	jwk, err := util.EcdsaToJWK(publicKey)
	require.NoError(t, err)
	require.Equal(t, jwk.KeyID, signedRequest.JWKThumbprint)
	require.True(t, result.PrivateKey.PublicKey.Equal(publicKey))
	require.Empty(t, result.Hostname)
	require.Equal(t, "cluster-id", result.OrchClusterID)

	client := fake.NewSimpleClientset()
	require.NoError(t, writeIdentitySecret(context.Background(), client, "default", "par-identity", result))
	// A follower reads the leader's secret through the existing coordination path.
	synctest.Test(t, func(t *testing.T) {
		secret, err := waitForLeaderAndSecret(context.Background(), make(chan struct{}), func() bool { return false }, client, "default", "par-identity", time.Second, acceptAnySecret)
		require.NoError(t, err)
		identity, err := parseSecretData(secret, "default", "par-identity")
		require.NoError(t, err)
		require.Equal(t, result.URN, identity.URN)
		require.Equal(t, result.OrchClusterID, identity.OrchClusterID)
		require.Equal(t, "test-org", identity.WIFOrgUUID)
		require.Equal(t, "datadoghq.eu", identity.WIFSite)
		require.Empty(t, identity.APIKeyHash)
		key, err := util.Base64ToJWK(identity.PrivateKey)
		require.NoError(t, err)
		require.True(t, result.PrivateKey.Equal(key.Key))
		require.False(t, ShouldReenroll(&AgentIdentifier{Hostname: "replica-b", OrchClusterID: "cluster-id"}, identity, "rotated-intake-key"))
	})
}

func TestClusterWIFEnrollmentRequiresClusterID(t *testing.T) {
	flavor.SetTestFlavor(t, flavor.ClusterAgent)
	auth := &delegatedauthmock.Mock{GetWorkloadTokenFunc: func(context.Context, string, common.WorkloadTokenRequest) (string, error) {
		t.Fatal("must reject missing cluster identity before requesting a token")
		return "", nil
	}}
	_, err := EnrollWithWorkloadToken(context.Background(), coreconfig.NewMock(t), auth, &AgentIdentifier{Hostname: "replica-a"})
	require.ErrorContains(t, err, "orchestrator cluster ID")
}

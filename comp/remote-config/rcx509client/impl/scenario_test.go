// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcx509clientimpl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	delegatedauthimpl "github.com/DataDog/datadog-agent/comp/core/delegatedauth/impl"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	rcsecretsv1 "github.com/DataDog/libdd-rc/ffi-hosts/go/rcproto/magic_tunnel/remote_config/rc_secrets"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// TestRCWorkloadScenarioAgent runs only in a dedicated manual test process.
// The configured provider, ETS HTTP client and RC component are real; the
// loopback bridge substitutes for Magic Tunnel and a product delivery handler.
func TestRCWorkloadScenarioAgent(t *testing.T) {
	issuerURL := os.Getenv("RC_WIF_SCENARIO_ETS_URL")
	if issuerURL == "" {
		t.Skip("manual combined scenario; see RC Secrets SCENARIO.md in dd-source")
	}
	backendURL := os.Getenv("RC_WIF_SCENARIO_RC_URL")
	for _, raw := range []string{issuerURL, backendURL} {
		parsed, err := url.Parse(raw)
		require.NoError(t, err)
		require.Equal(t, "https", parsed.Scheme)
		require.Equal(t, "127.0.0.1", parsed.Hostname())
	}
	// Both fixture servers use self-signed certificates and are constrained to
	// loopback above. This transport is never used for deployed services.
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	post := func(endpoint string, data []byte) {
		t.Helper()
		response, err := client.Post(endpoint, "application/octet-stream", bytes.NewReader(data))
		require.NoError(t, err)
		defer response.Body.Close()
		require.Equal(t, http.StatusNoContent, response.StatusCode)
	}
	t.Cleanup(func() { post(issuerURL+"/stop", nil) })
	response, err := client.Get(issuerURL + "/fixture")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var fixture struct {
		OrgUUID string `json:"org_uuid"`
		Proof   string `json:"proof"`
	}
	require.NoError(t, json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&fixture))
	require.NoError(t, response.Body.Close())
	require.NotEmpty(t, fixture.OrgUUID)
	require.NotEmpty(t, fixture.Proof)
	path := filepath.Join(t.TempDir(), "oidc-token")
	writeProof := func(proof string) {
		t.Helper()
		require.NoError(t, os.WriteFile(path+".new", []byte(proof), 0600))
		require.NoError(t, os.Rename(path+".new", path))
	}
	writeProof(fixture.Proof)
	var config rcx509client.ClientConfig
	deps, lifecycle := newDependencies(t, map[string]interface{}{
		enabledSetting: true, secretsEnabledSetting: true, secretProductsSetting: []string{"scenario-product"},
		"dd_url": issuerURL, "skip_ssl_validation": true,
		"remote_configuration.rc_dd_url": backendURL, "remote_configuration.no_tls_validation": true,
	}, func(c rcx509client.ClientConfig) (rcx509client.Client, error) {
		config = c
		return newFakeClient(), nil
	})
	auth := delegatedauthimpl.NewComponent().Comp
	require.NoError(t, auth.AddInstance(context.Background(), delegatedauth.InstanceParams{
		Config: deps.Config, OrgUUID: fixture.OrgUUID, APIKeyConfigKey: "api_key", RefreshInterval: 60,
		ProviderConfig: &cloudauthconfig.OIDCProviderConfig{Source: "file", TokenFile: path, OrgUUIDSource: "claims"},
	}))
	// This dedicated process ends before the configured instance's hourly
	// refresh. No production credentials or background service are created.
	deps.DelegatedAuth = auth
	component := New(deps).Comp
	require.Equal(t, rcx509client.StateInitialized, component.State())
	require.NotNil(t, config.PrepareSecretDelivery)
	require.NoError(t, lifecycle.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, lifecycle.Stop(context.Background())) })
	prepared, rejected, delivered := 0, 0, 0
	for {
		response, err := client.Get(backendURL + "/next")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		var packet struct {
			Kind      string `json:"kind"`
			Case      string `json:"case"`
			Data      []byte `json:"data"`
			ContextID string `json:"context_id"`
		}
		require.NoError(t, json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&packet))
		require.NoError(t, response.Body.Close())
		switch packet.Kind {
		case "prepare":
			proof := fixture.Proof
			if packet.Case == "invalid-proof" {
				parts := strings.Split(proof, ".")
				require.Len(t, parts, 3)
				replacement := "A"
				if strings.HasPrefix(parts[2], replacement) {
					replacement = "B"
				}
				parts[2] = replacement + parts[2][1:]
				proof = strings.Join(parts, ".")
			}
			writeProof(proof)
			raw, err := config.PrepareSecretDelivery(context.Background(), uint64(prepared+1), packet.Data)
			require.NoError(t, err)
			var reply rcsecretsv1.PrepareSecretDeliveryResponse
			require.NoError(t, proto.Unmarshal(raw, &reply))
			if packet.Case == "invalid-proof" {
				require.Equal(t, rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_IDENTITY_UNAVAILABLE, reply.Status)
				require.Empty(t, reply.Token)
				require.Empty(t, reply.PublicKey)
			} else {
				require.Equal(t, rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_OK, reply.Status)
				require.NotEmpty(t, reply.Token)
				require.Len(t, reply.PublicKey, 32)
			}
			require.Equal(t, "scenario-only-no-access", deps.Config.GetString("api_key"))
			post(backendURL+"/reply", raw)
			prepared++
		case "rejected":
			require.Contains(t, []string{"invalid-proof", "wrong-org"}, packet.Case)
			rejected++
		case "deliver":
			require.Equal(t, "valid", packet.Case)
			plaintext, err := component.OpenSecret("scenario-product", packet.ContextID, packet.Data)
			require.NoError(t, err)
			require.Equal(t, "rc-secrets-prototype-only", string(plaintext))
			_, err = component.OpenSecret("scenario-product", packet.ContextID, packet.Data)
			require.Error(t, err)
			digest := sha256.Sum256(plaintext)
			post(backendURL+"/ack", []byte(hex.EncodeToString(digest[:])))
			delivered++
		case "done":
			require.Equal(t, 3, prepared)
			require.Equal(t, 2, rejected)
			require.Equal(t, 1, delivered)
			post(backendURL+"/done", nil)
			fmt.Println("RC_WIF_AGENT_VERIFIED=proof-exchange,invalid-proof,wrong-org,decryption,single-use")
			return
		default:
			t.Fatalf("unknown scenario operation %q", packet.Kind)
		}
	}
}

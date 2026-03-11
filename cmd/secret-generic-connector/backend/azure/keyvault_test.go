// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keyvaultMockClient is the struct we'll use to mock the Azure KeyVault client
// for unit tests. E2E tests should be written with the real client.
type keyvaultMockClient struct {
	secrets map[string]string
}

func (c *keyvaultMockClient) GetSecret(_ context.Context, secretName string, _ string) (*string, error) {
	if val, ok := c.secrets[secretName]; ok {
		return &val, nil
	}
	return nil, secret.ErrKeyNotFound
}

func TestKeyvaultBackend(t *testing.T) {
	mockClient := &keyvaultMockClient{
		secrets: map[string]string{
			"key1": "{\"user\":\"foo\",\"password\":\"bar\"}",
			"key2": "{\"foo\":\"bar\"}",
		},
	}
	getKeyvaultClient = func(_, _ string) (keyvaultClient, error) {
		return mockClient, nil
	}

	keyvaultBackendParams := map[string]interface{}{
		"backend_type": "azure.keyvault",
		"keyvaulturl":  "https://my-vault.vault.azure.com/",
		"clientid":     "123abc45-123a-abcd-123a-123abc456def",
	}
	keyvaultSecretsBackend, err := NewKeyVaultBackend(keyvaultBackendParams)
	assert.NoError(t, err)

	ctx := context.Background()
	// Top-level key will be fetched as json
	secretOutput := keyvaultSecretsBackend.GetSecretOutput(ctx, "key1")
	assert.Equal(t, "{\"user\":\"foo\",\"password\":\"bar\"}", *secretOutput.Value)

	// Index into secret json
	secretOutput = keyvaultSecretsBackend.GetSecretOutput(ctx, "key1;user")
	assert.Equal(t, "foo", *secretOutput.Value)

	secretOutput = keyvaultSecretsBackend.GetSecretOutput(ctx, "key3")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)
}

func TestKeyVaultBackend_issue39434(t *testing.T) {
	mockClient := &keyvaultMockClient{
		secrets: map[string]string{
			"key1": "{\\\"foo\\\":\\\"bar\\\"}",
		},
	}
	getKeyvaultClient = func(_, _ string) (keyvaultClient, error) {
		return mockClient, nil
	}

	keyvaultBackendParams := map[string]interface{}{
		"backend_type": "azure.keyvault",
		"clientid":     "123abc45-123a-abcd-123a-123abc456def",
	}
	keyvaultSecretsBackend, err := NewKeyVaultBackend(keyvaultBackendParams)
	assert.NoError(t, err)

	ctx := context.Background()
	// Top-level keys are not fetchable
	secretOutput := keyvaultSecretsBackend.GetSecretOutput(ctx, "key1")
	assert.Equal(t, "{\\\"foo\\\":\\\"bar\\\"}", *secretOutput.Value)

	// Index into secret json
	secretOutput = keyvaultSecretsBackend.GetSecretOutput(ctx, "key1;foo")
	assert.Equal(t, "bar", *secretOutput.Value)
}

// --- Workload Identity ---

func TestWorkloadIdentityToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "/test-tenant/oauth2/v2.0/token")
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "client_credentials", r.FormValue("grant_type"))
		assert.Equal(t, "urn:ietf:params:oauth:client-assertion-type:jwt-bearer", r.FormValue("client_assertion_type"))
		assert.Equal(t, "my-federated-jwt", r.FormValue("client_assertion"))
		assert.Equal(t, "test-client", r.FormValue("client_id"))
		assert.Equal(t, "https://vault.azure.net/.default", r.FormValue("scope"))
		json.NewEncoder(w).Encode(map[string]string{"access_token": "wi-token-123"})
	}))
	defer srv.Close()

	t.Setenv("AZURE_TENANT_ID", "test-tenant")
	t.Setenv("AZURE_CLIENT_ID", "test-client")
	t.Setenv("AZURE_AUTHORITY_HOST", srv.URL)

	tokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("my-federated-jwt"), 0600))
	t.Setenv("AZURE_FEDERATED_TOKEN_FILE", tokenFile)

	c := &keyvaultHTTPClient{vaultURL: "https://vault.example.com"}
	token, err := c.workloadIdentityToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "wi-token-123", token)
}

func TestWorkloadIdentityToken_MissingEnvVars(t *testing.T) {
	t.Setenv("AZURE_TENANT_ID", "")
	t.Setenv("AZURE_CLIENT_ID", "")
	t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "")

	c := &keyvaultHTTPClient{}
	_, err := c.workloadIdentityToken(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not all set")
}

func TestWorkloadIdentityToken_MissingFile(t *testing.T) {
	t.Setenv("AZURE_TENANT_ID", "t")
	t.Setenv("AZURE_CLIENT_ID", "c")
	t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "/nonexistent/token")

	c := &keyvaultHTTPClient{}
	_, err := c.workloadIdentityToken(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read federated token file")
}

func TestWorkloadIdentityToken_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "bad request")
	}))
	defer srv.Close()

	t.Setenv("AZURE_TENANT_ID", "t")
	t.Setenv("AZURE_CLIENT_ID", "c")
	t.Setenv("AZURE_AUTHORITY_HOST", srv.URL)

	tokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("jwt"), 0600))
	t.Setenv("AZURE_FEDERATED_TOKEN_FILE", tokenFile)

	c := &keyvaultHTTPClient{}
	_, err := c.workloadIdentityToken(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}

// --- Azure CLI ---

func TestAzureCLIToken(t *testing.T) {
	old := azureExecCommand
	azureExecCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		assert.Equal(t, "az", name)
		return exec.CommandContext(ctx, "echo", `{"accessToken":"cli-token-456"}`)
	}
	defer func() { azureExecCommand = old }()

	c := &keyvaultHTTPClient{}
	token, err := c.azureCLIToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "cli-token-456", token)
}

func TestAzureCLIToken_CommandFails(t *testing.T) {
	old := azureExecCommand
	azureExecCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "false")
	}
	defer func() { azureExecCommand = old }()

	c := &keyvaultHTTPClient{}
	_, err := c.azureCLIToken(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "az CLI failed")
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// Package azure allows to fetch secrets from Azure keyvault service
package azure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
	"github.com/mitchellh/mapstructure"
)

// keyvaultClient is an interface for fetching secrets from Azure Key Vault.
// Tests provide a mock implementation.
type keyvaultClient interface {
	GetSecret(ctx context.Context, secretID string, secretVersion string) (value *string, err error)
}

// getKeyvaultClient is a variable that holds the function to create a new keyvaultClient.
// It is overwritten in tests.
var getKeyvaultClient = func(keyVaultURL, clientID string) (keyvaultClient, error) {
	return &keyvaultHTTPClient{
		vaultURL: strings.TrimRight(keyVaultURL, "/"),
		clientID: clientID,
	}, nil
}

// KeyVaultBackendConfig contains the configuration to connect for Azure backend
type KeyVaultBackendConfig struct {
	KeyVaultURL string `mapstructure:"keyvaulturl"`
	ClientID    string `mapstructure:"clientid"`
}

// KeyVaultBackend is a backend to fetch secrets from Azure
type KeyVaultBackend struct {
	Config KeyVaultBackendConfig
	Client keyvaultClient
}

// NewKeyVaultBackend returns a new backend for Azure
func NewKeyVaultBackend(bc map[string]interface{}) (*KeyVaultBackend, error) {
	backendConfig := KeyVaultBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	client, err := getKeyvaultClient(backendConfig.KeyVaultURL, backendConfig.ClientID)
	if err != nil {
		return nil, err
	}

	backend := &KeyVaultBackend{
		Config: backendConfig,
		Client: client,
	}
	return backend, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *KeyVaultBackend) GetSecretOutput(ctx context.Context, secretName string) secret.Output {
	var secretID, secretKey string

	sections := strings.SplitN(secretName, ";", 2)
	if len(sections) == 1 {
		secretID = sections[0]
	} else {
		secretID = sections[0]
		secretKey = sections[1]
	}

	version := ""
	value, err := b.Client.GetSecret(ctx, secretID, version)
	if err != nil {
		return b.makeErrorResponse(err)
	}

	// no semi-colon, return the secret value as a flat string
	if secretKey == "" {
		return secret.Output{Value: value, Error: nil}
	}

	// secret value is treated as structured json
	secretValue := make(map[string]string, 0)
	err = json.Unmarshal([]byte(*value), &secretValue)
	if err == nil {
		if val, ok := secretValue[secretKey]; ok {
			return secret.Output{Value: &val, Error: nil}
		}
	}

	// See https://github.com/Azure/azure-sdk-for-net/issues/39434, Azure KeyVault can return an escaped string value
	// that is not parsable as is. We need to unquote it first.
	unquoted, err := strconv.Unquote(fmt.Sprintf(`"%s"`, *value))
	if err == nil {
		err = json.Unmarshal([]byte(unquoted), &secretValue)
		if err == nil {
			if val, ok := secretValue[secretKey]; ok {
				return secret.Output{Value: &val, Error: nil}
			}
		}
	}

	return b.makeErrorResponse(errors.New("value does not contain secret key"))
}

func (b *KeyVaultBackend) makeErrorResponse(err error) secret.Output {
	es := err.Error()
	return secret.Output{Value: nil, Error: &es}
}

// --- Raw HTTP implementation of Azure Key Vault ---

const (
	kvAPIVersion = "7.4"
	// Azure IMDS endpoint for fetching managed identity tokens.
	azureIMDSEndpoint = "http://169.254.169.254/metadata/identity/oauth2/token"
)

type keyvaultHTTPClient struct {
	vaultURL string
	clientID string
}

// kvSecretBundle is the response from the Key Vault GetSecret API.
type kvSecretBundle struct {
	Value *string `json:"value"`
	ID    *string `json:"id"`
}

func (c *keyvaultHTTPClient) GetSecret(ctx context.Context, secretName, secretVersion string) (*string, error) {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Azure access token: %w", err)
	}

	secretURL := fmt.Sprintf("%s/secrets/%s", c.vaultURL, url.PathEscape(secretName))
	if secretVersion != "" {
		secretURL += "/" + url.PathEscape(secretVersion)
	}
	secretURL += "?api-version=" + kvAPIVersion

	req, err := http.NewRequestWithContext(ctx, "GET", secretURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Key Vault GetSecret returned %d: %s", resp.StatusCode, string(body))
	}

	var result kvSecretBundle
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Key Vault response: %w", err)
	}

	return result.Value, nil
}

// getAccessToken retrieves an OAuth2 bearer token for the Key Vault resource.
// It tries (in order):
//  1. Environment variables (AZURE_CLIENT_ID + AZURE_CLIENT_SECRET + AZURE_TENANT_ID)
//  2. Workload Identity (AZURE_FEDERATED_TOKEN_FILE)
//  3. Azure IMDS managed identity (with optional client_id from config)
//  4. Azure CLI
func (c *keyvaultHTTPClient) getAccessToken(ctx context.Context) (string, error) {
	// 1. Try environment variables first (service principal / client credentials).
	if token, err := c.clientCredentialsToken(ctx); err == nil {
		return token, nil
	}

	// 2. Try Workload Identity (federated token).
	if token, err := c.workloadIdentityToken(ctx); err == nil {
		return token, nil
	}

	// 3. Try IMDS managed identity.
	if token, err := c.imdsToken(ctx); err == nil {
		return token, nil
	}

	// 4. Try Azure CLI (dev convenience, last resort).
	if token, err := c.azureCLIToken(ctx); err == nil {
		return token, nil
	}

	return "", errors.New("unable to obtain Azure access token: tried client credentials, workload identity, IMDS managed identity, and Azure CLI")
}

// imdsToken fetches a token from the Azure Instance Metadata Service (managed identity).
func (c *keyvaultHTTPClient) imdsToken(ctx context.Context) (string, error) {
	params := url.Values{
		"api-version": {"2018-02-01"},
		"resource":    {"https://vault.azure.net"},
	}
	if c.clientID != "" {
		params.Set("client_id", c.clientID)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", azureIMDSEndpoint+"?"+params.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata", "true")

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("IMDS token request returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	if tokenResp.AccessToken == "" {
		return "", errors.New("empty access token from IMDS")
	}
	return tokenResp.AccessToken, nil
}

// workloadIdentityToken gets a token using Azure Workload Identity (Kubernetes federated token).
// Requires AZURE_TENANT_ID, AZURE_CLIENT_ID, and AZURE_FEDERATED_TOKEN_FILE.
func (c *keyvaultHTTPClient) workloadIdentityToken(ctx context.Context) (string, error) {
	tenantID := os.Getenv("AZURE_TENANT_ID")
	clientID := os.Getenv("AZURE_CLIENT_ID")
	tokenFile := os.Getenv("AZURE_FEDERATED_TOKEN_FILE")
	if tenantID == "" || clientID == "" || tokenFile == "" {
		return "", errors.New("AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_FEDERATED_TOKEN_FILE not all set")
	}

	tokenBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", fmt.Errorf("failed to read federated token file: %w", err)
	}
	assertion := strings.TrimSpace(string(tokenBytes))

	authority := os.Getenv("AZURE_AUTHORITY_HOST")
	if authority == "" {
		authority = "https://login.microsoftonline.com"
	}

	tokenURL := fmt.Sprintf("%s/%s/oauth2/v2.0/token", strings.TrimRight(authority, "/"), url.PathEscape(tenantID))

	data := url.Values{
		"grant_type":            {"client_credentials"},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {assertion},
		"client_id":             {clientID},
		"scope":                 {"https://vault.azure.net/.default"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("workload identity token request returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	if tokenResp.AccessToken == "" {
		return "", errors.New("empty access token from workload identity")
	}
	return tokenResp.AccessToken, nil
}

// azureExecCommand can be overridden in tests.
var azureExecCommand = exec.CommandContext

// azureCLIToken gets a token by invoking the Azure CLI.
func (c *keyvaultHTTPClient) azureCLIToken(ctx context.Context) (string, error) {
	cmd := azureExecCommand(ctx, "az", "account", "get-access-token", "--resource", "https://vault.azure.net", "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("az CLI failed: %w", err)
	}

	var result struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("failed to parse az CLI output: %w", err)
	}
	if result.AccessToken == "" {
		return "", errors.New("empty access token from az CLI")
	}
	return result.AccessToken, nil
}

// clientCredentialsToken gets a token using AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID.
func (c *keyvaultHTTPClient) clientCredentialsToken(ctx context.Context) (string, error) {
	tenantID := os.Getenv("AZURE_TENANT_ID")
	clientID := os.Getenv("AZURE_CLIENT_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	if tenantID == "" || clientID == "" || clientSecret == "" {
		return "", errors.New("AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET not all set")
	}

	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", url.PathEscape(tenantID))

	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"scope":         {"https://vault.azure.net/.default"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token request returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	if tokenResp.AccessToken == "" {
		return "", errors.New("empty access token")
	}
	return tokenResp.AccessToken, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// Package hashicorp allows to fetch secrets from Hashicorp vault service
package hashicorp

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/internal/awsutil"
	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
	"github.com/mitchellh/mapstructure"
	"github.com/qri-io/jsonpointer"
)

const implicitAuthToken = "implicit-auth"

// VaultSessionBackendConfig is the configuration for a Hashicorp vault backend
type VaultSessionBackendConfig struct {
	VaultRoleID              string `mapstructure:"vault_role_id"`
	VaultSecretID            string `mapstructure:"vault_secret_id"`
	VaultUserName            string `mapstructure:"vault_username"`
	VaultPassword            string `mapstructure:"vault_password"`
	VaultLDAPUserName        string `mapstructure:"vault_ldap_username"`
	VaultLDAPPassword        string `mapstructure:"vault_ldap_password"`
	VaultAuthType            string `mapstructure:"vault_auth_type"`
	VaultAWSRole             string `mapstructure:"vault_aws_role"`
	AWSRegion                string `mapstructure:"aws_region"`
	VaultKubernetesRole      string `mapstructure:"vault_kubernetes_role"`
	VaultKubernetesJWT       string `mapstructure:"vault_kubernetes_jwt"`
	VaultKubernetesJWTPath   string `mapstructure:"vault_kubernetes_jwt_path"`
	VaultKubernetesMountPath string `mapstructure:"vault_kubernetes_mount_path"`
	ImplicitAuth             string `mapstructure:"implicit_auth"`
}

// VaultBackendConfig contains the configuration to connect to Hashicorp vault backend
type VaultBackendConfig struct {
	VaultSession VaultSessionBackendConfig `mapstructure:"vault_session"`
	VaultToken   string                    `mapstructure:"vault_token"`
	VaultAddress string                    `mapstructure:"vault_address"`
	VaultTLS     *VaultTLSConfig           `mapstructure:"vault_tls_config"`
}

// VaultTLSConfig contains the TLS and certificate configuration
type VaultTLSConfig struct {
	CACert     string `mapstructure:"ca_cert"`
	CAPath     string `mapstructure:"ca_path"`
	ClientCert string `mapstructure:"client_cert"`
	ClientKey  string `mapstructure:"client_key"`
	TLSServer  string `mapstructure:"tls_server"`
	Insecure   bool   `mapstructure:"insecure"`
}

// vaultResponse mirrors the JSON structure returned by Vault's API.
type vaultResponse struct {
	RequestID     string                 `json:"request_id"`
	LeaseID       string                 `json:"lease_id"`
	LeaseDuration int                    `json:"lease_duration"`
	Renewable     bool                   `json:"renewable"`
	Data          map[string]interface{} `json:"data"`
	Warnings      []string               `json:"warnings"`
	Auth          *vaultAuth             `json:"auth"`
	Errors        []string               `json:"errors"`
}

type vaultAuth struct {
	ClientToken string `json:"client_token"`
}

type vaultMountOutput struct {
	Type    string            `json:"type"`
	Options map[string]string `json:"options"`
}

// vaultClient is a lightweight HTTP client for the Vault REST API.
type vaultClient struct {
	address    string
	token      string
	httpClient *http.Client
}

func (c *vaultClient) do(req *http.Request) (*vaultResponse, error) {
	if c.token != "" {
		req.Header.Set("X-Vault-Token", c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read vault response: %w", err)
	}

	// Vault returns 404 with no body for missing secrets.
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if len(body) == 0 {
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("vault request failed with status %d", resp.StatusCode)
		}
		return &vaultResponse{}, nil
	}

	var vr vaultResponse
	if err := json.Unmarshal(body, &vr); err != nil {
		return nil, fmt.Errorf("failed to decode vault response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if len(vr.Errors) > 0 {
			return nil, fmt.Errorf("vault request failed: %s", strings.Join(vr.Errors, ", "))
		}
		return nil, fmt.Errorf("vault request failed with status %d", resp.StatusCode)
	}

	return &vr, nil
}

func (c *vaultClient) read(ctx context.Context, path string) (*vaultResponse, error) {
	url := c.address + "/v1/" + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *vaultClient) write(ctx context.Context, path string, data map[string]interface{}) (*vaultResponse, error) {
	var body io.Reader
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to encode vault request body: %w", err)
		}
		body = bytes.NewReader(b)
	}
	url := c.address + "/v1/" + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.do(req)
}

func (c *vaultClient) listMounts(ctx context.Context) (map[string]*vaultMountOutput, error) {
	url := c.address + "/v1/sys/mounts"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("X-Vault-Token", c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read vault mounts response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("vault sys/mounts request failed with status %d", resp.StatusCode)
	}

	// The /v1/sys/mounts response wraps mounts under a "data" key when using
	// certain auth modes, but typically returns them at the top level. We try
	// the top-level parse first, then fall back to a wrapper.
	mounts := make(map[string]*vaultMountOutput)
	if err := json.Unmarshal(body, &mounts); err != nil {
		// Try wrapped format: {"data": { ... }}
		var wrapped struct {
			Data map[string]*vaultMountOutput `json:"data"`
		}
		if err2 := json.Unmarshal(body, &wrapped); err2 != nil {
			return nil, fmt.Errorf("failed to decode vault mounts response: %w", err)
		}
		mounts = wrapped.Data
	}

	return mounts, nil
}

func buildTLSConfig(tlsCfg *VaultTLSConfig) (*tls.Config, error) {
	tc := &tls.Config{} //nolint:gosec

	if tlsCfg.CACert != "" || tlsCfg.CAPath != "" {
		pool := x509.NewCertPool()

		if tlsCfg.CACert != "" {
			pem, err := os.ReadFile(tlsCfg.CACert)
			if err != nil {
				return nil, fmt.Errorf("failed to read CA cert %s: %w", tlsCfg.CACert, err)
			}
			if !pool.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("failed to parse CA cert %s", tlsCfg.CACert)
			}
		}

		if tlsCfg.CAPath != "" {
			entries, err := os.ReadDir(tlsCfg.CAPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read CA path %s: %w", tlsCfg.CAPath, err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				pem, err := os.ReadFile(filepath.Join(tlsCfg.CAPath, entry.Name()))
				if err != nil {
					return nil, fmt.Errorf("failed to read CA file %s: %w", entry.Name(), err)
				}
				pool.AppendCertsFromPEM(pem)
			}
		}

		tc.RootCAs = pool
	}

	if tlsCfg.ClientCert != "" && tlsCfg.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(tlsCfg.ClientCert, tlsCfg.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert/key: %w", err)
		}
		tc.Certificates = []tls.Certificate{cert}
	}

	if tlsCfg.TLSServer != "" {
		tc.ServerName = tlsCfg.TLSServer
	}

	if tlsCfg.Insecure {
		tc.InsecureSkipVerify = true //nolint:gosec
	}

	return tc, nil
}

// VaultBackend is a backend to fetch secrets from Hashicorp vault
type VaultBackend struct {
	Config VaultBackendConfig
	client *vaultClient
}

func getKubernetesJWTToken(sessionConfig VaultSessionBackendConfig) (string, error) {
	if sessionConfig.VaultKubernetesJWT != "" {
		return sessionConfig.VaultKubernetesJWT, nil
	}

	tokenPath := os.Getenv("DD_SECRETS_SA_TOKEN_PATH")
	if tokenPath == "" {
		if configPath := sessionConfig.VaultKubernetesJWTPath; configPath != "" {
			tokenPath = configPath
		} else {
			tokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
		}
	}

	tokenBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", fmt.Errorf("failed to read JWT token from %s: %w", tokenPath, err)
	}

	return strings.TrimSpace(string(tokenBytes)), nil
}

// newAuthenticationFromBackendConfig authenticates to Vault and returns the
// client token to use. The returned token is set on the vaultClient directly.
func newAuthenticationFromBackendConfig(bc VaultBackendConfig, client *vaultClient) (string, error) {
	sessionConfig := bc.VaultSession

	implicitAuthRaw := os.Getenv("DD_SECRETS_IMPLICIT_AUTH")
	if implicitAuthRaw == "" {
		implicitAuthRaw = sessionConfig.ImplicitAuth
	}
	if slices.Contains([]string{"true", "t", "1"}, strings.ToLower(implicitAuthRaw)) {
		return implicitAuthToken, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// AppRole auth
	if sessionConfig.VaultRoleID != "" && sessionConfig.VaultSecretID != "" {
		resp, err := client.write(ctx, "auth/approle/login", map[string]interface{}{
			"role_id":   sessionConfig.VaultRoleID,
			"secret_id": sessionConfig.VaultSecretID,
		})
		if err != nil {
			return "", fmt.Errorf("approle login failed: %w", err)
		}
		if resp == nil || resp.Auth == nil || resp.Auth.ClientToken == "" {
			return "", errors.New("approle login returned no token")
		}
		return resp.Auth.ClientToken, nil
	}

	// Userpass auth
	if sessionConfig.VaultUserName != "" && sessionConfig.VaultPassword != "" {
		resp, err := client.write(ctx, "auth/userpass/login/"+sessionConfig.VaultUserName, map[string]interface{}{
			"password": sessionConfig.VaultPassword,
		})
		if err != nil {
			return "", fmt.Errorf("userpass login failed: %w", err)
		}
		if resp == nil || resp.Auth == nil || resp.Auth.ClientToken == "" {
			return "", errors.New("userpass login returned no token")
		}
		return resp.Auth.ClientToken, nil
	}

	// LDAP auth
	if sessionConfig.VaultLDAPUserName != "" && sessionConfig.VaultLDAPPassword != "" {
		resp, err := client.write(ctx, "auth/ldap/login/"+sessionConfig.VaultLDAPUserName, map[string]interface{}{
			"password": sessionConfig.VaultLDAPPassword,
		})
		if err != nil {
			return "", fmt.Errorf("ldap login failed: %w", err)
		}
		if resp == nil || resp.Auth == nil || resp.Auth.ClientToken == "" {
			return "", errors.New("ldap login returned no token")
		}
		return resp.Auth.ClientToken, nil
	}

	// AWS auth
	if sessionConfig.VaultAuthType == "aws" && sessionConfig.VaultAWSRole != "" {
		token, err := vaultAWSLogin(ctx, client, sessionConfig.VaultAWSRole, sessionConfig.AWSRegion)
		if err != nil {
			return "", err
		}
		return token, nil
	}

	// Kubernetes auth
	if sessionConfig.VaultAuthType == "kubernetes" {
		role := os.Getenv("DD_SECRETS_VAULT_ROLE")
		if role == "" {
			role = sessionConfig.VaultKubernetesRole
		}
		if role == "" {
			return "", errors.New("kubernetes role not specified")
		}

		jwtToken, err := getKubernetesJWTToken(sessionConfig)
		if err != nil {
			return "", fmt.Errorf("failed to get Kubernetes JWT token: %w", err)
		}

		authPath := os.Getenv("DD_SECRETS_VAULT_AUTH_PATH")
		if authPath == "" {
			authPath = sessionConfig.VaultKubernetesMountPath
		}

		resp, err := client.write(ctx, authPath, map[string]interface{}{
			"jwt":  jwtToken,
			"role": role,
		})
		if err != nil {
			return "", fmt.Errorf("failed to authenticate to Vault: %w", err)
		}
		if resp == nil || resp.Auth == nil || resp.Auth.ClientToken == "" {
			return "", errors.New("vault login response did not return a token")
		}
		return resp.Auth.ClientToken, nil
	}

	// No session-based auth configured — return empty token, caller will
	// use a static token from config.
	return "", nil
}

// NewVaultBackend returns a new backend for Hashicorp vault
func NewVaultBackend(bc map[string]interface{}) (*VaultBackend, error) {
	backendConfig := VaultBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	vaultAddress := os.Getenv("VAULT_ADDR")
	if vaultAddress == "" {
		if configPath := backendConfig.VaultAddress; configPath != "" {
			vaultAddress = configPath
		} else {
			return nil, fmt.Errorf("failed to provide a vault address: %s", err)
		}
	}

	httpClient := &http.Client{}

	if backendConfig.VaultTLS != nil {
		tc, err := buildTLSConfig(backendConfig.VaultTLS)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize vault tls configuration: %s", err)
		}
		httpClient.Transport = &http.Transport{TLSClientConfig: tc}
	}

	client := &vaultClient{
		address:    vaultAddress,
		httpClient: httpClient,
	}

	authToken, err := newAuthenticationFromBackendConfig(backendConfig, client)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize vault authentication: %w", err)
	}

	if authToken != "" && authToken != implicitAuthToken {
		client.token = authToken
	} else if authToken != implicitAuthToken {
		if backendConfig.VaultToken != "" {
			client.token = backendConfig.VaultToken
		} else {
			return nil, errors.New("no auth method or token provided")
		}
	}

	return &VaultBackend{
		Config: backendConfig,
		client: client,
	}, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *VaultBackend) GetSecretOutput(ctx context.Context, secretString string) secret.Output {
	if strings.HasPrefix(secretString, "vault://") {
		return b.handleVaultURIFormat(ctx, secretString)
	}

	return b.handleTypicalFormat(ctx, secretString)
}

func (b *VaultBackend) handleVaultURIFormat(ctx context.Context, secretString string) secret.Output {
	pathWithKey := strings.TrimPrefix(secretString, "vault://")
	parts := strings.SplitN(pathWithKey, "#", 2)
	if len(parts) != 2 {
		es := "invalid vault:// format, expected 'vault://path#/json/pointer'"
		return secret.Output{Value: nil, Error: &es}
	}

	secretPath := parts[0]
	pointerStr := parts[1]

	if secretPath == "" {
		es := "secret path cannot be empty"
		return secret.Output{Value: nil, Error: &es}
	}

	if pointerStr == "" {
		es := "invalid JSON pointer"
		return secret.Output{Value: nil, Error: &es}
	}

	pointer, err := jsonpointer.Parse(pointerStr)
	if err != nil {
		es := fmt.Sprintf("invalid JSON pointer %q: %s", pointerStr, err.Error())
		return secret.Output{Value: nil, Error: &es}
	}

	sec, err := b.client.read(ctx, secretPath)
	if err != nil {
		es := err.Error()
		return secret.Output{Value: nil, Error: &es}
	}

	if sec == nil || sec.Data == nil {
		es := "secret data is nil"
		return secret.Output{Value: nil, Error: &es}
	}

	head := pointer.Head()
	if head == nil {
		es := "invalid JSON pointer"
		return secret.Output{Value: nil, Error: &es}
	}

	var value interface{}
	switch *head {
	case "data":
		// Handle /data or /data/... paths
		if len(pointer) == 1 {
			// Just "/data" - return the appropriate data structure
			if dataField, ok := sec.Data["data"]; ok {
				// KV v2: return the nested data field
				value = dataField
			} else {
				// KV v1: return the entire data map
				value = sec.Data
			}
		} else {
			// "/data/something" - need to evaluate the rest
			tail := pointer.Tail()
			if tail == nil {
				es := "invalid JSON pointer structure"
				return secret.Output{Value: nil, Error: &es}
			}

			if dataField, ok := sec.Data["data"].(map[string]interface{}); ok {
				// This is likely KV v2, evaluate the tail against the nested data
				value, err = tail.Eval(dataField)
			} else {
				// This is likely KV v1, evaluate the tail against sec.Data directly
				value, err = tail.Eval(sec.Data)
			}
			if err != nil {
				es := fmt.Sprintf("no value found for pointer %s", pointer)
				return secret.Output{Value: nil, Error: &es}
			}
		}
	case "lease_duration":
		value = sec.LeaseDuration
	case "lease_id":
		value = sec.LeaseID
	case "renewable":
		value = sec.Renewable
	case "request_id":
		value = sec.RequestID
	case "warnings":
		value = sec.Warnings
	default:
		// For other keys, try to evaluate directly against sec.Data
		// This handles KV v1 where data is stored directly, not under a "data" key
		value, err = pointer.Eval(sec.Data)
		if err != nil {
			es := fmt.Sprintf("no value found for pointer %s", pointer)
			return secret.Output{Value: nil, Error: &es}
		}
	}

	if value == nil {
		es := fmt.Sprintf("no value found for pointer %s", pointer)
		return secret.Output{Value: nil, Error: &es}
	}

	valueStr := fmt.Sprintf("%v", value)
	return secret.Output{Value: &valueStr, Error: nil}
}

func (b *VaultBackend) handleTypicalFormat(ctx context.Context, secretString string) secret.Output {
	segments := strings.SplitN(secretString, ";", 2)
	if len(segments) != 2 {
		es := "invalid secret format, expected 'secret_path;key' or 'vault://path#/json/pointer'"
		return secret.Output{Value: nil, Error: &es}
	}
	secretPath := segments[0]
	secretKey := segments[1]

	// KV version detection:
	// If the mount path is set as /Example/Path, and the secret path is set at /Example/Path/Secret,
	// then we need to query from /Example/Path/data/Secret in kv v2, and /Example/Path/Secret in kv v1.
	isKVv2, mountPrefix := b.isKVv2Mount(ctx, secretPath)

	readPath := secretPath
	if isKVv2 {
		readPath = insertDataPath(secretPath, mountPrefix)
	}

	sec, err := b.client.read(ctx, readPath)
	if err != nil {
		es := err.Error()
		return secret.Output{Value: nil, Error: &es}
	}

	if sec == nil || sec.Data == nil {
		es := "secret data is nil"
		return secret.Output{Value: nil, Error: &es}
	}

	var dataMap map[string]interface{}
	if isKVv2 {
		if inner, ok := sec.Data["data"].(map[string]interface{}); ok {
			dataMap = inner
		} else {
			es := "secret data is not in expected format for KV v2"
			return secret.Output{Value: nil, Error: &es}
		}
	} else {
		dataMap = sec.Data
	}

	if dataMap == nil {
		es := "There is no actual data in the secret"
		return secret.Output{Value: nil, Error: &es}
	}

	if data, ok := dataMap[secretKey]; ok {
		if strValue, ok := data.(string); ok {
			return secret.Output{Value: &strValue, Error: nil}
		}
		es := "secret value is not a string"
		return secret.Output{Value: nil, Error: &es}
	}

	es := secret.ErrKeyNotFound.Error()
	return secret.Output{Value: nil, Error: &es}
}

func (b *VaultBackend) isKVv2Mount(ctx context.Context, secretPath string) (bool, string) {
	mounts, err := b.client.listMounts(ctx)
	if err != nil {
		return false, ""
	}

	cleanPath := strings.TrimPrefix(secretPath, "/")
	parts := strings.Split(cleanPath, "/")

	// Try progressively longer prefixes: Datadog/, Datadog/Production/, etc.
	for i := 1; i <= len(parts); i++ {
		prefix := strings.Join(parts[:i], "/") + "/"

		if mountInfo, ok := mounts[prefix]; ok {
			if mountInfo.Type == "kv" {
				version := mountInfo.Options["version"]
				return version == "2", prefix
			}
		}
	}

	// If no mount was found, then assume that it is v1 of the Hashicorp vault secrets engine
	return false, ""
}

func insertDataPath(secretPath, mountPrefix string) string {

	trimmedSecret := strings.TrimPrefix(secretPath, "/")
	trimmedMount := strings.TrimPrefix(mountPrefix, "/")

	if !strings.HasPrefix(trimmedSecret, trimmedMount) {
		// secret path does not match mount prefix, so we are skipping data insertion
		return secretPath
	}

	// remove mount prefix from path
	relative := strings.TrimPrefix(trimmedSecret, trimmedMount)
	relative = strings.TrimPrefix(relative, "/")

	if relative == "" {
		return trimmedMount + "data"
	}
	return trimmedMount + "data/" + relative
}

// vaultAWSLogin performs AWS IAM auth against Vault using raw STS
// GetCallerIdentity signed with AWS SigV4.
func vaultAWSLogin(ctx context.Context, client *vaultClient, role, region string) (string, error) {
	cfg, err := awsutil.ResolveConfig(ctx, awsutil.SessionConfig{
		Region: region,
	})
	if err != nil {
		return "", fmt.Errorf("failed to resolve AWS credentials for Vault auth: %w", err)
	}

	stsRegion := cfg.Region
	if stsRegion == "" {
		stsRegion = "us-east-1"
	}

	body := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	endpoint := fmt.Sprintf("https://sts.%s.amazonaws.com/", stsRegion)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	awsutil.SignRequest(req, cfg.Credentials, stsRegion, "sts", body)

	headersJSON, err := json.Marshal(req.Header)
	if err != nil {
		return "", err
	}

	loginData := map[string]interface{}{
		"role":                    role,
		"iam_http_request_method": "POST",
		"iam_request_url":         base64.StdEncoding.EncodeToString([]byte(endpoint)),
		"iam_request_body":        base64.StdEncoding.EncodeToString(body),
		"iam_request_headers":     base64.StdEncoding.EncodeToString(headersJSON),
	}

	resp, err := client.write(ctx, "auth/aws/login", loginData)
	if err != nil {
		return "", fmt.Errorf("vault AWS login failed: %w", err)
	}
	if resp == nil || resp.Auth == nil || resp.Auth.ClientToken == "" {
		return "", errors.New("vault AWS login returned no token")
	}
	return resp.Auth.ClientToken, nil
}

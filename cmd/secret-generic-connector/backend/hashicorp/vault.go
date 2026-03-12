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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/internal/awsutil"
	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/internal/vaulthttp"
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

// VaultBackend is a backend to fetch secrets from Hashicorp vault
type VaultBackend struct {
	Config VaultBackendConfig
	Client *vaulthttp.Client
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

// authenticateVaultClient performs authentication against Vault and sets the
// client token. It returns the token string (or implicitAuthToken sentinel).
func authenticateVaultClient(ctx context.Context, bc VaultBackendConfig, client *vaulthttp.Client) (string, error) {
	sessionConfig := bc.VaultSession

	implicitAuthRaw := os.Getenv("DD_SECRETS_IMPLICIT_AUTH")
	if implicitAuthRaw == "" {
		implicitAuthRaw = sessionConfig.ImplicitAuth
	}
	if slices.Contains([]string{"true", "t", "1"}, strings.ToLower(implicitAuthRaw)) {
		return implicitAuthToken, nil
	}

	// AppRole
	if sessionConfig.VaultRoleID != "" && sessionConfig.VaultSecretID != "" {
		sec, err := client.Write(ctx, "auth/approle/login", map[string]interface{}{
			"role_id":   sessionConfig.VaultRoleID,
			"secret_id": sessionConfig.VaultSecretID,
		})
		if err != nil {
			return "", fmt.Errorf("approle login failed: %w", err)
		}
		token, err := sec.TokenID()
		if err != nil {
			return "", fmt.Errorf("approle: unable to extract token: %w", err)
		}
		return token, nil
	}

	// Userpass
	if sessionConfig.VaultUserName != "" && sessionConfig.VaultPassword != "" {
		sec, err := client.Write(ctx, "auth/userpass/login/"+sessionConfig.VaultUserName, map[string]interface{}{
			"password": sessionConfig.VaultPassword,
		})
		if err != nil {
			return "", fmt.Errorf("userpass login failed: %w", err)
		}
		token, err := sec.TokenID()
		if err != nil {
			return "", fmt.Errorf("userpass: unable to extract token: %w", err)
		}
		return token, nil
	}

	// LDAP
	if sessionConfig.VaultLDAPUserName != "" && sessionConfig.VaultLDAPPassword != "" {
		sec, err := client.Write(ctx, "auth/ldap/login/"+sessionConfig.VaultLDAPUserName, map[string]interface{}{
			"password": sessionConfig.VaultLDAPPassword,
		})
		if err != nil {
			return "", fmt.Errorf("ldap login failed: %w", err)
		}
		token, err := sec.TokenID()
		if err != nil {
			return "", fmt.Errorf("ldap: unable to extract token: %w", err)
		}
		return token, nil
	}

	// AWS
	if sessionConfig.VaultAuthType == "aws" && sessionConfig.VaultAWSRole != "" {
		return vaultAWSLogin(ctx, client, sessionConfig.VaultAWSRole, sessionConfig.AWSRegion)
	}

	// Kubernetes
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

		sec, err := client.Write(ctx, authPath, map[string]interface{}{
			"jwt":  jwtToken,
			"role": role,
		})
		if err != nil {
			return "", fmt.Errorf("failed to authenticate to Vault: %w", err)
		}

		token, err := sec.TokenID()
		if err != nil {
			return "", fmt.Errorf("unable to extract token from Vault login response: %w", err)
		}
		if token == "" {
			return "", errors.New("vault login response did not return a token")
		}

		return token, nil
	}

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

	var tlsCfg *vaulthttp.TLSConfig
	if backendConfig.VaultTLS != nil {
		tlsCfg = &vaulthttp.TLSConfig{
			CACert:        backendConfig.VaultTLS.CACert,
			CAPath:        backendConfig.VaultTLS.CAPath,
			ClientCert:    backendConfig.VaultTLS.ClientCert,
			ClientKey:     backendConfig.VaultTLS.ClientKey,
			TLSServerName: backendConfig.VaultTLS.TLSServer,
			Insecure:      backendConfig.VaultTLS.Insecure,
		}
	}

	client, err := vaulthttp.NewClient(vaultAddress, tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %s", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	authToken, err := authenticateVaultClient(ctx, backendConfig, client)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize vault authentication: %w", err)
	}

	if authToken != "" && authToken != implicitAuthToken {
		client.SetToken(authToken)
	} else if authToken != implicitAuthToken {
		if backendConfig.VaultToken != "" {
			client.SetToken(backendConfig.VaultToken)
		} else {
			return nil, errors.New("no auth method or token provided")
		}
	}

	return &VaultBackend{
		Config: backendConfig,
		Client: client,
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

	sec, err := b.Client.Read(ctx, secretPath)
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
	isKVv2, mountPrefix := isKVv2Mount(ctx, b.Client, secretPath)

	readPath := secretPath
	if isKVv2 {
		readPath = insertDataPath(secretPath, mountPrefix)
	}

	sec, err := b.Client.Read(ctx, readPath)
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

func isKVv2Mount(ctx context.Context, client *vaulthttp.Client, secretPath string) (bool, string) {
	mounts, err := client.ListMounts(ctx)
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
func vaultAWSLogin(ctx context.Context, client *vaulthttp.Client, role, region string) (string, error) {
	var options []func(*config.LoadOptions) error
	if region != "" {
		options = append(options, func(o *config.LoadOptions) error {
			o.Region = region
			return nil
		})
	}
	cfg, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config for Vault auth: %w", err)
	}

	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve AWS credentials for Vault auth: %w", err)
	}

	stsRegion := cfg.Region
	if stsRegion == "" {
		stsRegion = "us-east-1"
	}

	// Build and sign a GetCallerIdentity request.
	body := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	endpoint := fmt.Sprintf("https://sts.%s.amazonaws.com/", stsRegion)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	awsutil.SignRequest(req, creds, stsRegion, "sts", body)

	// Encode the signed request for Vault.
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

	sec, err := client.Write(ctx, "auth/aws/login", loginData)
	if err != nil {
		return "", fmt.Errorf("aws auth login failed: %w", err)
	}

	token, err := sec.TokenID()
	if err != nil {
		return "", fmt.Errorf("aws auth: unable to extract token: %w", err)
	}
	return token, nil
}

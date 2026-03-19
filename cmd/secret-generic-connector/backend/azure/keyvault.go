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
	"os"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
	"github.com/mitchellh/mapstructure"
)

// keyvaultClient is an interface that defines the methods we use from the ssm client
// As the AWS SDK doesn't provide a real mock, we'll have to make our own that
// matches this interface
type keyvaultClient interface {
	GetSecret(ctx context.Context, secretID string, secretVersion string, opt *azsecrets.GetSecretOptions) (result azsecrets.GetSecretResponse, err error)
}

// getKeyvaultClient is a variable that holds the function to create a new keyvaultClient
// it will be overwritten in tests
var getKeyvaultClient = func(cfg KeyVaultBackendConfig) (keyvaultClient, error) {
	s := cfg.AzureSession
	var err error
	var cred azcore.TokenCredential

	switch {
	case s.AzureTenantID != "" && s.AzureClientID != "" && s.AzureClientSecret != "":
		cred, err = azidentity.NewClientSecretCredential(s.AzureTenantID, s.AzureClientID, s.AzureClientSecret, nil)
		if err != nil {
			return nil, fmt.Errorf("getting client secret credentials: %s", err)
		}
	case s.AzureTenantID != "" && s.AzureClientID != "" && s.AzureClientCertificatePath != "":
		certData, err := os.ReadFile(s.AzureClientCertificatePath)
		if err != nil {
			return nil, fmt.Errorf("reading certificate file: %s", err)
		}
		var password []byte
		if s.AzureClientCertificatePassword != "" {
			password = []byte(s.AzureClientCertificatePassword)
		}
		certs, key, err := azidentity.ParseCertificates(certData, password)
		if err != nil {
			return nil, fmt.Errorf("parsing certificate: %s", err)
		}
		opts := &azidentity.ClientCertificateCredentialOptions{
			SendCertificateChain: s.AzureClientSendCertificateChain,
		}
		cred, err = azidentity.NewClientCertificateCredential(s.AzureTenantID, s.AzureClientID, certs, key, opts)
		if err != nil {
			return nil, fmt.Errorf("getting client certificate credentials: %s", err)
		}
	case s.AzureClientID != "":
		opts := azidentity.ManagedIdentityCredentialOptions{ID: azidentity.ClientID(s.AzureClientID)}
		cred, err = azidentity.NewManagedIdentityCredential(&opts)
		if err != nil {
			return nil, fmt.Errorf("getting identity credentials: %s", err)
		}
	default:
		cred, err = azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("could not get default credentials: %s", err)
		}
	}

	client, err := azsecrets.NewClient(cfg.KeyVaultURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}
	return client, nil
}

// AzureSessionBackendConfig is the session configuration for Azure (nested under azure_session).
type AzureSessionBackendConfig struct {
	AzureClientID                   string `mapstructure:"azure_client_id"`
	AzureTenantID                   string `mapstructure:"azure_tenant_id"`
	AzureClientSecret               string `mapstructure:"azure_client_secret"`
	AzureClientCertificatePath      string `mapstructure:"azure_client_certificate_path"`
	AzureClientCertificatePassword  string `mapstructure:"azure_client_certificate_password"`
	AzureClientSendCertificateChain bool   `mapstructure:"azure_client_send_certificate_chain"`
}

// KeyVaultBackendConfig contains the configuration to connect for Azure backend
type KeyVaultBackendConfig struct {
	KeyVaultURL  string                    `mapstructure:"keyvaulturl"`
	AzureSession AzureSessionBackendConfig `mapstructure:"azure_session"`
}

// KeyVaultBackend is a backend to fetch secrets from Azure
type KeyVaultBackend struct {
	Config KeyVaultBackendConfig
	Client keyvaultClient
}

// normalizeAzureSession returns the azure_session map, normalizing map[interface{}]interface{}
// (produced by yaml.v2) to map[string]interface{}, and creating it if absent.
func normalizeAzureSession(bc map[string]interface{}) map[string]interface{} {
	if bc["azure_session"] == nil {
		m := make(map[string]interface{})
		bc["azure_session"] = m
		return m
	}
	if m, ok := bc["azure_session"].(map[string]interface{}); ok {
		return m
	}
	if m, ok := bc["azure_session"].(map[interface{}]interface{}); ok {
		out := make(map[string]interface{}, len(m))
		for k, v := range m {
			if s, ok := k.(string); ok {
				out[s] = v
			}
		}
		bc["azure_session"] = out
		return out
	}
	return nil
}

// NewKeyVaultBackend returns a new backend for Azure
func NewKeyVaultBackend(bc map[string]interface{}) (*KeyVaultBackend, error) {
	azureSession := normalizeAzureSession(bc)

	// Accept top-level "clientid" as alias for azure_session.azure_client_id
	if v, ok := bc["clientid"]; ok && azureSession != nil {
		if _, set := azureSession["azure_client_id"]; !set {
			azureSession["azure_client_id"] = v
		}
	}

	backendConfig := KeyVaultBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	client, err := getKeyvaultClient(backendConfig)
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
	out, err := b.Client.GetSecret(ctx, secretID, version, nil)
	if err != nil {
		return b.makeErrorResponse(err)
	}

	// no semi-colon, return the secret value as a flat string
	if secretKey == "" {
		return secret.Output{Value: out.Value, Error: nil}
	}

	// secret value is treated as structured json
	secretValue := make(map[string]string, 0)
	err = json.Unmarshal([]byte(*out.Value), &secretValue)
	if err == nil {
		if val, ok := secretValue[secretKey]; ok {
			return secret.Output{Value: &val, Error: nil}
		}
	}

	// See https://github.com/Azure/azure-sdk-for-net/issues/39434, Azure KeyVault can return an escaped string value
	// that is not parsable as is. We need to unquote it first.
	unquoted, err := strconv.Unquote(fmt.Sprintf(`"%s"`, *out.Value))
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

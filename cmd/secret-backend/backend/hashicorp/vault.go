// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// Package hashicorp allows to fetch secrets from Hashicorp vault service
package hashicorp

import (
	"context"
	"errors"
	"strings"

	"github.com/DataDog/datadog-secret-backend/secret"
	"github.com/hashicorp/vault/api"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog/log"
)

// VaultBackendConfig contains the configuration to connect to Hashicorp vault backend
type VaultBackendConfig struct {
	VaultSession VaultSessionBackendConfig `mapstructure:"vault_session"`
	VaultToken   string                    `mapstructure:"vault_token"`
	BackendType  string                    `mapstructure:"backend_type"`
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
	Client *api.Client
}

// NewVaultBackend returns a new backend for Hashicorp vault
func NewVaultBackend(bc map[string]interface{}) (*VaultBackend, error) {
	backendConfig := VaultBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		log.Error().Err(err).
			Msg("failed to map backend configuration")
		return nil, err
	}

	clientConfig := &api.Config{Address: backendConfig.VaultAddress}

	if backendConfig.VaultTLS != nil {
		tlsConfig := &api.TLSConfig{
			CACert:        backendConfig.VaultTLS.CACert,
			CAPath:        backendConfig.VaultTLS.CAPath,
			ClientCert:    backendConfig.VaultTLS.ClientCert,
			ClientKey:     backendConfig.VaultTLS.ClientKey,
			TLSServerName: backendConfig.VaultTLS.TLSServer,
			Insecure:      backendConfig.VaultTLS.Insecure,
		}
		err := clientConfig.ConfigureTLS(tlsConfig)
		if err != nil {
			log.Error().Err(err).
				Msg("failed to initialize vault tls configuration")
			return nil, err
		}
	}

	client, err := api.NewClient(clientConfig)
	if err != nil {
		log.Error().Err(err).
			Msg("failed to create vault client")
		return nil, err
	}

	authMethod, err := NewVaultConfigFromBackendConfig(backendConfig.VaultSession)
	if err != nil {
		log.Error().Err(err).
			Msg("failed to initialize vault session")
		return nil, err
	}
	if authMethod != nil {
		authInfo, err := client.Auth().Login(context.TODO(), authMethod)
		if err != nil {
			log.Error().Err(err).
				Msg("failed to created auth info")
			return nil, err
		}
		if authInfo == nil {
			log.Error().Err(err).
				Msg("No auth info returned")
			return nil, errors.New("no auth info returned")
		}
	} else if backendConfig.VaultToken != "" {
		client.SetToken(backendConfig.VaultToken)
	} else {
		log.Error().
			Msg("No auth method or token provided")
		return nil, errors.New("no auth method or token provided")
	}

	backend := &VaultBackend{
		Config: backendConfig,
		Client: client,
	}
	return backend, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *VaultBackend) GetSecretOutput(secretString string) secret.Output {
	segments := strings.SplitN(secretString, ";", 2)
	if len(segments) != 2 {
		es := "invalid secret format, expected 'secret_id;key'"
		log.Error().
			Str("backend_type", b.Config.BackendType).
			Str("secret_string", secretString).
			Msg(es)
		return secret.Output{Value: nil, Error: &es}
	}
	secretPath := segments[0]
	secretKey := segments[1]
	sec, err := b.Client.Logical().Read(secretPath)
	if err != nil {
		es := err.Error()
		log.Error().Err(err).
			Str("backend_type", b.Config.BackendType).
			Str("secret_path", secretPath).
			Str("secret_key", secretKey).
			Msg("failed to read secret from vault")
		return secret.Output{Value: nil, Error: &es}
	}

	if sec == nil || sec.Data == nil {
		es := "secret data is nil"
		log.Error().
			Str("backend_type", b.Config.BackendType).
			Str("secret_path", secretPath).
			Str("secret_key", secretKey).
			Msg(es)
		return secret.Output{Value: nil, Error: &es}
	}

	if data, ok := sec.Data[secretKey]; ok {
		if strValue, ok := data.(string); ok {
			return secret.Output{Value: &strValue, Error: nil}
		}
		es := "secret value is not a string"
		log.Error().
			Str("backend_type", b.Config.BackendType).
			Str("secret_path", secretPath).
			Str("secret_key", secretKey).
			Msg(es)
		return secret.Output{Value: nil, Error: &es}
	}

	es := secret.ErrKeyNotFound.Error()
	log.Error().
		Str("backend_type", b.Config.BackendType).
		Str("secret_path", secretPath).
		Str("secret_key", secretKey).
		Msg("failed to retrieve secrets")
	return secret.Output{Value: nil, Error: &es}
}

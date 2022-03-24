package vault

import (
	"context"
	"errors"

	"github.com/mitchellh/mapstructure"
	"github.com/rapdev-io/datadog-secret-backend/secret"
	"github.com/rs/zerolog/log"
)

type VaultBackendConfig struct {
	VaultSession VaultSessionBackendConfig `mapstructure:"vault_session"`
	BackendType  string                    `mapstructure:"backend_type"`
	VaultAddress string                    `mapstructure:"vault_address"`
	SecretPath   string                    `mapstructure:"secret_path"`
	Secrets      []string                  `mapstructure:"secrets"`
}

type VaultBackend struct {
	BackendId string
	Config    VaultBackendConfig
	Secret    map[string]string
}

func NewVaultBackend(backendId string, bc map[string]interface{}) (*VaultBackend, error) {
	backendConfig := VaultBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendId).
			Msg("failed to map backend configuration")
		return nil, err
	}

	authMethod, client, err := NewVaultConfigFromBackendConfig(backendId, backendConfig.VaultSession, backendConfig.VaultAddress)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendId).
			Msg("failed to initialize vault session")
		return nil, err
	}

	authInfo, err := client.Auth().Login(context.TODO(), authMethod)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendId).
			Msg("failed to created auth info")
		return nil, err
	}
	if authInfo == nil {
		log.Error().Err(err).Str("backend_id", backendId).
			Msg("No auth info returned")
		return nil, errors.New("No auth info returned")
	}

	secret, err := client.Logical().Read(backendConfig.SecretPath)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendId).
			Msg("Failed to read secret")
		return nil, err
	}
	secretValue := make(map[string]string, 0)

	if backendConfig.SecretPath != "" {
		if len(backendConfig.Secrets) > 0 {
			for _, item := range backendConfig.Secrets {
				secretValue[item] = secret.Data[item].(string)
			}
		}
	}

	backend := &VaultBackend{
		BackendId: backendId,
		Config:    backendConfig,
		Secret:    secretValue,
	}
	return backend, nil
}

func (b *VaultBackend) GetSecretOutput(secretKey string) secret.SecretOutput {
	if val, ok := b.Secret[secretKey]; ok {
		return secret.SecretOutput{Value: &val, Error: nil}
	}
	es := errors.New("backend does not provide secret key").Error()
	
	log.Error().
		Str("backend_id",   b.BackendId).
		Str("backend_type", b.Config.BackendType).
		Strs("secrets",     b.Config.Secrets).
		Str("secret_path",  b.Config.SecretPath).
		Str("secret_key",   secretKey).
		Msg("failed to retrieve secrets")	
	return secret.SecretOutput{Value: nil, Error: &es}
}
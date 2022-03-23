package vault

import (
	"errors"

	"github.com/mitchellh/mapstructure"
	"github.com/rapdev-io/datadog-secret-backend/secret"
	log "github.com/sirupsen/logrus"
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
		log.WithError(err).Error("failed to map backend configuration")
		return nil, err
	}

	cfg, err := NewVaultConfigFromBackendConfig(backendId, backendConfig.VaultSession)
	if err != nil {
		log.WithFields(log.Fields{
			"backend_id": backendId,
		}).WithError(err).Error("failed to initialize vault session")
		return nil, err
	}

	secret, err := cfg.Read(backendConfig.SecretPath)
	if err != nil {
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
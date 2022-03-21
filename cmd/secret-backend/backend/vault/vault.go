package vault

import (
	"context"
	"errors"
	"github.com/hashicorp/vault/api"
	"github.com/mitchellh/mapstructure"
	"github.com/rapdev-io/datadog-secret-backend/secret"
	log "github.com/sirupsen/logrus"
)

type VaultBackendConfig struct {
	VaultSession VaultSessionBackendConfig `mapstructure:"vault_session"`
	BackendType  string                    `mapstructure:"backend_type"`
	SecretID     string                    `mapstructure:"secret_id"`
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

	token, err := *cfg.TokenID()
	if err != nil {
		log.WithError(err).Error("failed to get token for given secret")
	}
	log.WithFields(log.Fields{
		"token": token
	})
}
package vault

import (
	"context"
	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/api/auth"
	"github.com/hashicorp/vault/api/auth/approle"
)

type VaultAddress string `mapstructure:"vault_address"`

type VaultSessionBackendConfig struct {
	VaultRoleId   string         `mapstructure:"vault_role_id"`
	VaultSecretId string         `mapstructure:"vault_secret_id"`
	VaultUserName string         `mapstructure:"vault_username"`
	VaultPassword string         `mapstructure:"vault_password"`
}

var VaultSecretID = *approle.SecretID

func NewVaultConfigFromBackendConfig(backendId string, sessionConfig VaultSessionBackendConfig) (*api.Secret, error) {
	vaultClient, err := CreateVaultClient()
	if err != nil {
		return nil, err
	}

	
	if sessionConfig.VaultRoleId != "" {
		options := make([]func(*approle.LoginOption) error, 0)
		options = append(options, func(o *approle.LoginOption) error {
			o.roleID = sessionConfig.VaultRoleId
			o.secretID = sessionConfig.VaultSecretId
		})

		cfg, err := approle.NewAppRoleAuth(options)
		if err != nil {
			return nil, err
		}
	}

	if sessionConfig.VaultUserName != "" {
		options := make([]func(*userpass.LoginOption) error, 0)
		options = append(options, func(o *userpass.LoginOption) error {
			o.username = sessionConfig.VaultUserName
			o.password = sessionConfig.VaultPassword
		})

		cfg, err := userpass.NewUserPass(options)
		if err != nil {
			return nil, err
		}

	}

	apiSecret, err := cfg.Login(ctx.Background(), vaultClient)
	if err != nil {
		return nil, err
	}
	
	return &apiSecret, err

}

func CreateVaultClient() (*api.Client, error) {
	conf := api.DefaultConfig()
	client, err := api.NewClient(conf)
	if err != nil {
		return nil, err
	}
	err = client.SetAddress(VaultAddress)
	if err != nil {
		return nil, err
	}

	return *client, nil
}

func DetectAuthMethod() ()
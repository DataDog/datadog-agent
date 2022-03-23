package vault

import (
	"context"

	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/api/auth/approle"
	"github.com/hashicorp/vault/api/auth/userpass"
)

type VaultAddress string         `mapstructure:"vault_address"`

type VaultSessionBackendConfig struct {
	VaultRoleId   string         `mapstructure:"vault_role_id"`
	VaultSecretId string         `mapstructure:"vault_secret_id"`
	VaultUserName string         `mapstructure:"vault_username"`
	VaultPassword string         `mapstructure:"vault_password"`
	VaultTLS      VaultTLSConfig `mapstructure:"tls_config"`
}

type VaultTLSConfig struct {
	CACert     string `mapstructure:"ca_cert"`
	CAPath     string `mapstructure:"ca_path"`
	ClientCert string `mapstructure:"client_cert"`
	ClientKey  string `mapstructure:"client_key"`
	TLSServer  string `mapstructure:"tls_server"`
	Insecure   bool   `mapstructure:"insecure"`
}

func NewVaultConfigFromBackendConfig(backendId string, sessionConfig VaultSessionBackendConfig) (*api.Logical, error) {
	config := &api.Config{Address: VaultAddress}
	if sessionConfig.VaultTLS != nil {
		tlsConfig := &api.TLSConfig{
			CACert:        sessionConfig.VaultTLS.CACert,
			CAPath:        sessionConfig.VaultTLS.CAPath,
			ClientCert:    sessionConfig.VaultTLS.ClientCert,
			ClientKey:     sessionConfig.VaultTLS.ClientKey,
			TLSServerName: sessionConfig.VaultTLS.TLSServer,
			Insecure:      sessionConfig.VaultTLS.Insecure,
		}
		err := config.ConfigureTLS(tlsConfig)
		if err != nil {
			return nil, err
		}
	}
	
	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	if sessionConfig.VaultRoleId != "" {
		if sessionConfig.VaultSecretId != "" {
			secretId := &approle.SecretID{FromString: sessionConfig.VaultSecretId}
			auth, err := approle.NewAppRoleAuth(sessionConfig.VaultRoleId, secretId)
			if err != nil {
				return nil, err
			}
		}
	}

	if sessionConfig.VaultUserName != "" {
		if sessionConfig.VaultPassword != "" {
			password := &userpass.Password{FromString: sessionConfig.VaultPassword}
			auth, err := userpass.NewUserpassAuth(sessionConfig.VaultUserName, password)
			if err != nil {
				return nil, err
			}
		}
	}

	authInfo, err := client.Auth().Login(context.TODO(), auth)
	if err != nil {
		return nil, err
	}
	if authInfo == nil {
		return nil, errors.New("No auth info returned")
	}

	logical, err := client.Logical()
	if err != nil {
		return nil, err
	}
	return &logical, err
}
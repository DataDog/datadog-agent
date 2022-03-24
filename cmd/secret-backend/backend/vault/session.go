package vault

import (
	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/api/auth/approle"
	"github.com/hashicorp/vault/api/auth/ldap"
	"github.com/hashicorp/vault/api/auth/userpass"
)

type VaultSessionBackendConfig struct {
	VaultRoleId       string          `mapstructure:"vault_role_id"`
	VaultSecretId     string          `mapstructure:"vault_secret_id"`
	VaultUserName     string          `mapstructure:"vault_username"`
	VaultPassword     string          `mapstructure:"vault_password"`
	VaultLDAPUserName string          `mapstructure:"vault_ldap_username"`
	VaultLDAPPassword string          `mapstructure:"vault_ldap_password"`
	VaultTLS          *VaultTLSConfig `mapstructure:"tls_config,omitempty"`
}

type VaultTLSConfig struct {
	CACert     string `mapstructure:"ca_cert"`
	CAPath     string `mapstructure:"ca_path"`
	ClientCert string `mapstructure:"client_cert"`
	ClientKey  string `mapstructure:"client_key"`
	TLSServer  string `mapstructure:"tls_server"`
	Insecure   bool   `mapstructure:"insecure"`
}

func NewVaultConfigFromBackendConfig(backendId string, sessionConfig VaultSessionBackendConfig, vaultAddress string) (api.AuthMethod, *api.Client, error) {
	config := &api.Config{Address: vaultAddress}
	var auth api.AuthMethod
	
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
			return nil, nil, err
		}
	}
	
	client, err := api.NewClient(config)
	if err != nil {
		return nil, nil, err
	}

	if sessionConfig.VaultRoleId != "" {
		if sessionConfig.VaultSecretId != "" {
			secretId := &approle.SecretID{FromString: sessionConfig.VaultSecretId}
			auth, err = approle.NewAppRoleAuth(sessionConfig.VaultRoleId, secretId)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	if sessionConfig.VaultUserName != "" {
		if sessionConfig.VaultPassword != "" {
			password := &userpass.Password{FromString: sessionConfig.VaultPassword}
			auth, err = userpass.NewUserpassAuth(sessionConfig.VaultUserName, password)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	if sessionConfig.VaultLDAPUserName != "" {
		if sessionConfig.VaultLDAPPassword != "" {
			password := &ldap.Password{FromString: sessionConfig.VaultLDAPPassword}
			auth, err = ldap.NewLDAPAuth(sessionConfig.VaultLDAPUserName, password)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	return auth, client, err
}
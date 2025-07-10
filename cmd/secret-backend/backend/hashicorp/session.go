// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package hashicorp

import (
	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/api/auth/approle"
	"github.com/hashicorp/vault/api/auth/aws"
	"github.com/hashicorp/vault/api/auth/ldap"
	"github.com/hashicorp/vault/api/auth/userpass"
)

// VaultSessionBackendConfig is the configuration for a Hashicorp vault backend
type VaultSessionBackendConfig struct {
	VaultRoleID       string `mapstructure:"vault_role_id"`
	VaultSecretID     string `mapstructure:"vault_secret_id"`
	VaultUserName     string `mapstructure:"vault_username"`
	VaultPassword     string `mapstructure:"vault_password"`
	VaultLDAPUserName string `mapstructure:"vault_ldap_username"`
	VaultLDAPPassword string `mapstructure:"vault_ldap_password"`
	VaultAuthType     string `mapstructure:"vault_auth_type"`
	VaultAWSRole      string `mapstructure:"vault_aws_role"`
	AWSRegion         string `mapstructure:"aws_region"`
}

// NewVaultConfigFromBackendConfig returns a AuthMethod for Hashicorp vault based on the configuration
func NewVaultConfigFromBackendConfig(sessionConfig VaultSessionBackendConfig) (api.AuthMethod, error) {
	var auth api.AuthMethod
	var err error
	if sessionConfig.VaultRoleID != "" {
		if sessionConfig.VaultSecretID != "" {
			secretID := &approle.SecretID{FromString: sessionConfig.VaultSecretID}
			auth, err = approle.NewAppRoleAuth(sessionConfig.VaultRoleID, secretID)
			if err != nil {
				return nil, err
			}
		}
	}

	if sessionConfig.VaultUserName != "" {
		if sessionConfig.VaultPassword != "" {
			password := &userpass.Password{FromString: sessionConfig.VaultPassword}
			auth, err = userpass.NewUserpassAuth(sessionConfig.VaultUserName, password)
			if err != nil {
				return nil, err
			}
		}
	}

	if sessionConfig.VaultLDAPUserName != "" {
		if sessionConfig.VaultLDAPPassword != "" {
			password := &ldap.Password{FromString: sessionConfig.VaultLDAPPassword}
			auth, err = ldap.NewLDAPAuth(sessionConfig.VaultLDAPUserName, password)
			if err != nil {
				return nil, err
			}
		}
	}

	if sessionConfig.VaultAuthType == "aws" && sessionConfig.VaultAWSRole != "" {
		opts := []aws.LoginOption{
			aws.WithIAMAuth(),
			aws.WithRole(sessionConfig.VaultAWSRole),
		}

		if sessionConfig.AWSRegion != "" {
			opts = append(opts, aws.WithRegion(sessionConfig.AWSRegion))
		}

		return aws.NewAWSAuth(opts...)
	}

	return auth, err
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package azure

import (
	"os"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
)

// SessionBackendConfig is the configuration for a Azure backend
type SessionBackendConfig struct {
	TenantID            string `mapstructure:"azure_tenant_id"`
	ClientID            string `mapstructure:"azure_client_id"`
	ClientSecret        string `mapstructure:"azure_client_secret"`
	CertificatePath     string `mapstructure:"azure_certificate_path"`
	CertificatePassword string `mapstructure:"azure_certificate_password"`
}

// NewConfigFromBackendConfig returns a Authorizer for Azure based on the configuration
func NewConfigFromBackendConfig(sessionConfig SessionBackendConfig) (*autorest.Authorizer, error) {
	if sessionConfig.TenantID != "" {
		os.Setenv("AZURE_TENANT_ID", sessionConfig.TenantID)
	}

	if sessionConfig.ClientID != "" {
		os.Setenv("AZURE_CLIENT_ID", sessionConfig.ClientID)
	}

	if sessionConfig.ClientSecret != "" {
		os.Setenv("AZURE_CLIENT_SECRET", sessionConfig.ClientSecret)
	}

	if sessionConfig.CertificatePath != "" {
		os.Setenv("AZURE_CERTIFICATE_PATH", sessionConfig.CertificatePath)
	}

	if sessionConfig.CertificatePassword != "" {
		os.Setenv("AZURE_CERTIFICATE_PASSWORD", sessionConfig.CertificatePassword)
	}

	os.Setenv("AZURE_AD_RESOURCE", "https://vault.azure.net")

	cfg, err := auth.NewAuthorizerFromEnvironment()
	return &cfg, err
}

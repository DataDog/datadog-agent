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

type AzureSessionBackendConfig struct {
	AzureTenantId            string `mapstructure:"azure_tenant_id"`
	AzureClientId            string `mapstructure:"azure_client_id"`
	AzureClientSecret        string `mapstructure:"azure_client_secret"`
	AzureCertificatePath     string `mapstructure:"azure_certificate_path"`
	AzureCertificatePassword string `mapstructure:"azure_certificate_password"`
}

func NewAzureConfigFromBackendConfig(backendId string, sessionConfig AzureSessionBackendConfig) (*autorest.Authorizer, error) {

	if sessionConfig.AzureTenantId != "" {
		os.Setenv("AZURE_TENANT_ID", sessionConfig.AzureTenantId)
	}

	if sessionConfig.AzureClientId != "" {
		os.Setenv("AZURE_CLIENT_ID", sessionConfig.AzureClientId)
	}

	if sessionConfig.AzureClientSecret != "" {
		os.Setenv("AZURE_CLIENT_SECRET", sessionConfig.AzureClientSecret)
	}

	if sessionConfig.AzureCertificatePath != "" {
		os.Setenv("AZURE_CERTIFICATE_PATH", sessionConfig.AzureCertificatePath)
	}

	if sessionConfig.AzureCertificatePassword != "" {
		os.Setenv("AZURE_CERTIFICATE_PASSWORD", sessionConfig.AzureCertificatePassword)
	}

	os.Setenv("AZURE_AD_RESOURCE", "https://vault.azure.net")

	cfg, err := auth.NewAuthorizerFromEnvironment()
	return &cfg, err
}

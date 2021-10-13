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
	AzureCertificatePath     string `mapstructure:"client_certificate"`
	AzureCertificatePassword string `mapstructure:"client_certificate_password"`
	AzureUsername            string `mapstructure:"azure_username"`
	AzurePassword            string `mapstructure:"azure_password"`
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

	if sessionConfig.AzureUsername != "" {
		os.Setenv("AZURE_USERNAME", sessionConfig.AzureUsername)
	}

	if sessionConfig.AzurePassword != "" {
		os.Setenv("AZURE_PASSWORD", sessionConfig.AzurePassword)
	}

	cfg, err := auth.NewAuthorizerFromEnvironment()
	return &cfg, err
}
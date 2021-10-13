package azure

type AwsKeyVaultBackendConfig struct {
	AzureSession AzureSessionBackendConfig   `mapstructure:"azure_session"`
	BackendType  string                      `mapstructure:"backend_type"`
	KeyVaultURL  string                      `mapstructure:"keyvaulturl"`
}

type AzureKeyVaultBackend struct {
	BackendId string
	Config    AzureKeyVaultBackendConfig
	Secret    map[string]string
}


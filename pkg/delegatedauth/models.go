package delegatedauth

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// DelegatedAuthConfig provides cloud provider based authentication configuration.
type DelegatedAuthConfig struct {
	OrgUUID      string
	Provider     string
	ProviderAuth DelegatedAuthProvider
}

// DelegatedAuthProvider is an interface for getting a delegated token utilizing different methods.
type DelegatedAuthProvider interface {
	GetApiKey(cfg pkgconfigmodel.Reader, config *DelegatedAuthConfig) (*string, error)
}

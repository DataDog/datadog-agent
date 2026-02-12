package agentprovider

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"go.opentelemetry.io/collector/confmap"
)

const (
	schemeName = "dd"
)

type agentProvider struct {
	config configManager
}

func NewFactory(agentConfig config.Component) confmap.ProviderFactory {
	return confmap.NewProviderFactory(newProvider(agentConfig))
}

func newProvider(agentConfig config.Component) confmap.CreateProviderFunc {
	return func(_ confmap.ProviderSettings) confmap.Provider {
		return &agentProvider{newConfigManager(agentConfig)}
	}
}

func (ap *agentProvider) Retrieve(_ context.Context, uri string, _ confmap.WatcherFunc) (*confmap.Retrieved, error) {
	if uri != "dd:" {
		return nil, fmt.Errorf("%q uri is not supported by %q provider", uri, schemeName)
	}
	if ap.config.config == nil {
		return nil, fmt.Errorf("agent config is not available")
	}

	if len(ap.config.endpoints) == 0 {
		return nil, fmt.Errorf("no valid endpoints configured: ensure Datadog agent configuration has 'api_key' and either 'apm_config.profiling_dd_url' or 'site' set")
	}

	stringMap := buildConfig(ap.config)

	return confmap.NewRetrieved(stringMap)
}

func (ap *agentProvider) Scheme() string {
	return schemeName
}

func (*agentProvider) Shutdown(context.Context) error {
	return nil
}

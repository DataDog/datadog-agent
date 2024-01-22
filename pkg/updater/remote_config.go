package updater

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type RemoteConfig struct {
	service *service.Service
	client  *client.Client
}

func NewRemoteConfig(hostname string) (*RemoteConfig, error) {
	service, err := common.NewRemoteConfigService(hostname)
	if err != nil {
		return nil, fmt.Errorf("unable to create rc service: %w", err)
	}
	client, err := client.NewClient(service, client.WithProducts(state.ProductUpdaterCatalogDD, state.ProductUpdaterAgent, state.ProductUpdaterTask), client.WithoutTufVerification())
	if err != nil {
		return nil, fmt.Errorf("unable to create rc client: %w", err)
	}
	service.Start(context.Background())
	client.Start()
	return &RemoteConfig{
		service: service,
		client:  client,
	}, nil
}

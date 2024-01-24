// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// RemoteConfig is a wrapper around the remote config service and client
type RemoteConfig struct {
	service *service.Service
	client  *client.Client
}

// NewRemoteConfig returns a new RemoteConfig instance
func NewRemoteConfig(hostname string) (*RemoteConfig, error) {
	service, err := common.NewRemoteConfigService(hostname, service.WithDatabaseFileName("remote-config-updater.db"))
	if err != nil {
		return nil, fmt.Errorf("unable to create rc service: %w", err)
	}
	client, err := client.NewClient(service, client.WithAgent("updater", "1.0"), client.WithProducts(state.ProductUpdaterCatalogDD, state.ProductUpdaterAgent, state.ProductUpdaterTask), client.WithoutTufVerification())
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

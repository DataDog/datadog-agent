// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package azurebackend provides some utility functions and types for operating
// with Azure services.
package azurebackend

import (
	"context"
	"net/http"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/agentless/types"

	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
)

var (
	globalConfigs   map[string]Config
	globalConfigsMu sync.Mutex
)

// A Config provides service configuration for service clients.
type Config struct {
	// The credentials object to use when signing requests.
	Credentials azcore.TokenCredential

	// The HTTP Client the SDK's API clients will use to invoke HTTP requests.
	HTTPClient *http.Client

	ComputeClientFactory *armcompute.ClientFactory

	// The subscription ID of the agentless scanner
	ScannerSubscription string

	// The location of the agentless scanner
	ScannerLocation string

	// The resource group used by resources created for and by the scanner
	ScannerResourceGroup string
}

// GetConfigFromCloudID returns the configuration for the Azure subscription of the given cloud ID.
func GetConfigFromCloudID(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, cloudID types.CloudID) (Config, error) {
	resourceID, err := cloudID.AsAzureID()
	if err != nil {
		return Config{}, err
	}

	return GetConfig(ctx, statsd, sc, resourceID.SubscriptionID)
}

// GetConfig returns the configuration for the given subscription ID.
func GetConfig(ctx context.Context, _ ddogstatsd.ClientInterface, sc *types.ScannerConfig, subscriptionID string) (Config, error) {
	globalConfigsMu.Lock()
	defer globalConfigsMu.Unlock()

	if cfg, ok := globalConfigs[subscriptionID]; ok {
		return cfg, nil
	}

	var cred azcore.TokenCredential
	var err error
	if len(sc.AzureClientID) != 0 {
		cred, err = azidentity.NewManagedIdentityCredential(&azidentity.ManagedIdentityCredentialOptions{
			ID: azidentity.ClientID(sc.AzureClientID),
		})
	} else {
		cred, err = azidentity.NewDefaultAzureCredential(nil)
	}
	if err != nil {
		return Config{}, err
	}

	metadata, err := GetInstanceMetadata(ctx)
	if err != nil {
		return Config{}, err
	}

	computeClientFactory, err := armcompute.NewClientFactory(subscriptionID, cred, nil)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Credentials:          cred,
		ComputeClientFactory: computeClientFactory,
		ScannerSubscription:  metadata.Compute.SubscriptionID,
		ScannerLocation:      metadata.Compute.Location,
		ScannerResourceGroup: metadata.Compute.ResourceGroupName,
	}
	if globalConfigs == nil {
		globalConfigs = make(map[string]Config)
	}
	globalConfigs[subscriptionID] = cfg
	return cfg, nil
}

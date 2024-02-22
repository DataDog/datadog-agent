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

	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
)

var (
	statsd *ddogstatsd.Client

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
func GetConfigFromCloudID(ctx context.Context, cloudID types.CloudID) (Config, error) {
	resourceID, err := cloudID.AsAzureID()
	if err != nil {
		return Config{}, err
	}

	return GetConfig(ctx, resourceID.SubscriptionID)
}

// GetConfig returns the configuration for the given subscription ID.
func GetConfig(ctx context.Context, subscriptionID string) (Config, error) {
	globalConfigsMu.Lock()
	defer globalConfigsMu.Unlock()

	if statsd == nil {
		statsd, _ = ddogstatsd.New("localhost:8125")
	}

	clientID := pkgconfig.Datadog.GetString("agentless_scanner.azure_client_id")
	var cred azcore.TokenCredential
	var err error
	if len(clientID) != 0 {
		cred, err = azidentity.NewManagedIdentityCredential(&azidentity.ManagedIdentityCredentialOptions{
			ID: azidentity.ClientID(clientID),
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

	return Config{
		Credentials:          cred,
		ComputeClientFactory: computeClientFactory,
		ScannerSubscription:  metadata.Compute.SubscriptionID,
		ScannerLocation:      metadata.Compute.Location,
		ScannerResourceGroup: metadata.Compute.ResourceGroupName,
	}, nil
}

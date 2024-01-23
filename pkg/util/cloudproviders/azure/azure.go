// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package azure provides utilities to detect Azure cloud provider.
package azure

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cachedfetch"
)

// declare these as vars not const to ease testing
var (
	metadataURL = "http://169.254.169.254"
	timeout     = 300 * time.Millisecond

	// CloudProviderName contains the inventory name of for EC2
	CloudProviderName = "Azure"
)

const hostnameStyleSetting = "azure_hostname_style"

// IsRunningOn returns true if the agent is running on Azure
func IsRunningOn(ctx context.Context) bool {
	panic("not called")
}

var vmIDFetcher = cachedfetch.Fetcher{
	Name: "Azure vmID",
	Attempt: func(ctx context.Context) (interface{}, error) {
		res, err := getResponseWithMaxLength(ctx,
			metadataURL+"/metadata/instance/compute/vmId?api-version=2017-04-02&format=text",
			config.Datadog.GetInt("metadata_endpoints_max_hostname_size"))
		if err != nil {
			return nil, fmt.Errorf("Azure HostAliases: unable to query metadata endpoint: %s", err)
		}
		return []string{res}, nil
	},
}

// GetHostAliases returns the VM ID from the Azure Metadata api
func GetHostAliases(ctx context.Context) ([]string, error) {
	panic("not called")
}

var resourceGroupNameFetcher = cachedfetch.Fetcher{
	Name: "Azure Cluster Name",
	Attempt: func(ctx context.Context) (interface{}, error) {
		rg, err := getResponse(ctx,
			metadataURL+"/metadata/instance/compute/resourceGroupName?api-version=2017-08-01&format=text")
		if err != nil {
			return "", fmt.Errorf("unable to query metadata endpoint: %s", err)
		}
		return rg, nil
	},
}

// GetClusterName returns the name of the cluster containing the current VM by parsing the resource group name.
// It expects the resource group name to have the format (MC|mc)_resource-group_cluster-name_zone
func GetClusterName(ctx context.Context) (string, error) {
	panic("not called")
}

// GetNTPHosts returns the NTP hosts for Azure if it is detected as the cloud provider, otherwise an empty array.
// Demo: https://docs.microsoft.com/en-us/azure/virtual-machines/linux/time-sync
func GetNTPHosts(ctx context.Context) []string {
	panic("not called")
}

func getResponseWithMaxLength(ctx context.Context, endpoint string, maxLength int) (string, error) {
	panic("not called")
}

func getResponse(ctx context.Context, url string) (string, error) {
	panic("not called")
}

// GetHostname returns hostname based on Azure instance metadata.
func GetHostname(ctx context.Context) (string, error) {
	panic("not called")
}

var instanceMetaFetcher = cachedfetch.Fetcher{
	Name: "Azure Instance Metadata",
	Attempt: func(ctx context.Context) (interface{}, error) {
		metadataJSON, err := getResponse(ctx,
			metadataURL+"/metadata/instance/compute?api-version=2017-08-01")
		if err != nil {
			return "", fmt.Errorf("failed to get Azure instance metadata: %s", err)
		}
		return metadataJSON, nil
	},
}

func getHostnameWithConfig(ctx context.Context, config config.Config) (string, error) {
	panic("not called")
}

var publicIPv4Fetcher = cachedfetch.Fetcher{
	Name: "Azure Public IP",
	Attempt: func(ctx context.Context) (interface{}, error) {
		publicIPv4, err := getResponse(ctx,
			metadataURL+"/metadata/instance/network/interface/0/ipv4/ipAddress/0/publicIpAddress?api-version=2017-04-02&format=text")
		if err != nil {
			return "", fmt.Errorf("failed to get Azure public ip: %s", err)
		}

		return publicIPv4, nil
	},
}

// GetPublicIPv4 returns the public IPv4 address of the current Azure instance
func GetPublicIPv4(ctx context.Context) (string, error) {
	panic("not called")
}

type instanceMetadata struct {
	SubscriptionID string `json:"subscriptionId"`
}

// GetSubscriptionID returns the subscription ID of the current Azure instance
func GetSubscriptionID(ctx context.Context) (string, error) {
	panic("not called")
}

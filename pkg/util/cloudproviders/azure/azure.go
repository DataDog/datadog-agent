// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package azure provides utilities to detect Azure cloud provider.
package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/cachedfetch"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// declare these as vars not const to ease testing
var (
	metadataURL = "http://169.254.169.254"
	timeout     = 300 * time.Millisecond

	// CloudProviderName contains the inventory name of for EC2
	CloudProviderName = "Azure"
)

const hostnameStyleSetting = "azure_hostname_style"
const aksManagedOrchestratorTag = "aks-managed-orchestrator"
const kubernetesTagValue = "Kubernetes"

type metadata struct {
	VMID              string
	Name              string
	ResourceGroupName string
	SubscriptionID    string
	TagsList          []map[string]string
	OsProfile         struct {
		ComputerName string
	}
}

func (metadata metadata) GetFromStyle(style string) (string, error) {
	switch style {
	case "vmid":
		return metadata.VMID, nil
	case "name":
		return metadata.Name, nil
	case "name_and_resource_group":
		return fmt.Sprintf("%s.%s", metadata.Name, metadata.ResourceGroupName), nil
	case "full":
		return fmt.Sprintf("%s.%s.%s", metadata.Name, metadata.ResourceGroupName, metadata.SubscriptionID), nil
	case "os_computer_name":
		return strings.ToLower(metadata.OsProfile.ComputerName), nil
	default:
		return "", fmt.Errorf("invalid azure_hostname_style value: %s", style)
	}
}

// IsRunningOn returns true if the agent is running on Azure
func IsRunningOn(ctx context.Context) bool {
	if _, err := GetHostAliases(ctx); err == nil {
		return true
	}
	return false
}

var vmIDFetcher = cachedfetch.Fetcher{
	Name: "Azure vmID",
	Attempt: func(ctx context.Context) (interface{}, error) {
		res, err := getResponseWithMaxLength(ctx,
			metadataURL+"/metadata/instance/compute/vmId?api-version=2017-04-02&format=text",
			pkgconfigsetup.Datadog().GetInt("metadata_endpoints_max_hostname_size"))
		if err != nil {
			return nil, fmt.Errorf("Azure HostAliases: unable to query metadata VM ID endpoint: %s", err)
		}
		return []string{res}, nil
	},
}

// GetHostAliases returns the VM ID from the Azure Metadata api
func GetHostAliases(ctx context.Context) ([]string, error) {
	aliases := []string{}
	vm, err := vmIDFetcher.FetchStringSlice(ctx)
	if err == nil {
		aliases = append(aliases, vm...)
	}

	metadata, err := getMetadata(ctx)
	if err != nil {
		return aliases, fmt.Errorf("Azure GetHostAliases: unable to query metadata endpoint: %s", err)
	}

	if isKubernetesTag(metadata.TagsList) {
		aliases = append(aliases, metadata.OsProfile.ComputerName)
	}
	return aliases, err
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
	all, err := resourceGroupNameFetcher.FetchString(ctx)
	if err != nil {
		return "", err
	}

	splitAll := strings.Split(all, "_")
	if len(splitAll) < 4 || strings.ToLower(splitAll[0]) != "mc" {
		return "", fmt.Errorf("cannot parse the clustername from resource group name: %s", all)
	}

	return splitAll[len(splitAll)-2], nil
}

// GetNTPHosts returns the NTP hosts for Azure if it is detected as the cloud provider, otherwise an empty array.
// Demo: https://docs.microsoft.com/en-us/azure/virtual-machines/linux/time-sync
func GetNTPHosts(ctx context.Context) []string {
	if IsRunningOn(ctx) {
		return []string{"time.windows.com"}
	}

	return nil
}

func getResponseWithMaxLength(ctx context.Context, endpoint string, maxLength int) (string, error) {
	result, err := getResponse(ctx, endpoint)
	if err != nil {
		return result, err
	}
	if len(result) > maxLength {
		return "", fmt.Errorf("%v gave a response with length > to %v", endpoint, maxLength)
	}
	return result, err
}

func getResponse(ctx context.Context, url string) (string, error) {
	if !pkgconfigsetup.IsCloudProviderEnabled(CloudProviderName, pkgconfigsetup.Datadog()) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}

	return httputils.Get(ctx, url, map[string]string{"Metadata": "true"}, timeout, pkgconfigsetup.Datadog())
}

// GetHostname returns hostname based on Azure instance metadata.
func GetHostname(ctx context.Context) (string, error) {
	return getHostnameWithConfig(ctx, pkgconfigsetup.Datadog())
}

var instanceMetaFetcher = cachedfetch.Fetcher{
	Name: "Azure Instance Metadata",
	Attempt: func(ctx context.Context) (interface{}, error) {
		metadataJSON, err := getResponse(ctx,
			metadataURL+"/metadata/instance/compute?api-version=2021-02-01")
		if err != nil {
			return "", fmt.Errorf("failed to get Azure instance metadata: %s", err)
		}
		return metadataJSON, nil
	},
}

func isKubernetesTag(tagsList []map[string]string) bool {
	for _, tag := range tagsList {
		if tag["name"] == aksManagedOrchestratorTag && strings.Contains(tag["value"], kubernetesTagValue) {
			return true
		}
	}
	return false
}

func getMetadata(ctx context.Context) (metadata, error) {
	metadataInfo := metadata{}
	metadataJSON, err := instanceMetaFetcher.FetchString(ctx)
	if err != nil {
		return metadataInfo, err
	}

	if err := json.Unmarshal([]byte(metadataJSON), &metadataInfo); err != nil {
		return metadataInfo, fmt.Errorf("failed to parse Azure instance metadata: %s", err)
	}
	return metadataInfo, nil
}

func getHostnameWithConfig(ctx context.Context, config model.Config) (string, error) {
	style := config.GetString(hostnameStyleSetting)
	metadata, err := getMetadata(ctx)
	if err != nil {
		return "", err
	}

	isKubernetes := isKubernetesTag(metadata.TagsList)

	if style == "os" {
		if isKubernetes {
			// If running in AKS, use the node name as the hostname
			style = "os_computer_name"
		} else {
			return "", fmt.Errorf("azure_hostname_style is set to 'os'")
		}
	}

	name, err := metadata.GetFromStyle(style)
	if err != nil {
		return "", err
	}

	if err := validate.ValidHostname(name); err != nil {
		return "", err
	}

	return name, nil
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
	return publicIPv4Fetcher.FetchString(ctx)
}

type instanceMetadata struct {
	SubscriptionID string `json:"subscriptionId"`
}

// GetSubscriptionID returns the subscription ID of the current Azure instance
func GetSubscriptionID(ctx context.Context) (string, error) {
	body, err := instanceMetaFetcher.FetchString(ctx)
	if err != nil {
		return "", err
	}

	var metadata instanceMetadata
	if err := json.Unmarshal([]byte(body), &metadata); err != nil {
		return "", err
	}

	return metadata.SubscriptionID, nil
}

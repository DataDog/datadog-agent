// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
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

// IsRunningOn returns true if the agent is running on Azure
func IsRunningOn(ctx context.Context) bool {
	if _, err := GetHostAlias(ctx); err == nil {
		return true
	}
	return false
}

// GetHostAlias returns the VM ID from the Azure Metadata api
func GetHostAlias(ctx context.Context) (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}
	res, err := getResponseWithMaxLength(ctx, metadataURL+"/metadata/instance/compute/vmId?api-version=2017-04-02&format=text",
		config.Datadog.GetInt("metadata_endpoints_max_hostname_size"))
	if err != nil {
		return "", fmt.Errorf("Azure HostAliases: unable to query metadata endpoint: %s", err)
	}
	return res, nil
}

// GetClusterName returns the name of the cluster containing the current VM by parsing the resource group name.
// It expects the resource group name to have the format (MC|mc)_resource-group_cluster-name_zone
func GetClusterName(ctx context.Context) (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}
	all, err := getResponse(ctx, metadataURL+"/metadata/instance/compute/resourceGroupName?api-version=2017-08-01&format=text")
	if err != nil {
		return "", fmt.Errorf("unable to query metadata endpoint: %s", err)
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
	client := http.Client{
		Transport: httputils.CreateHTTPTransport(),
		Timeout:   timeout,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Add("Metadata", "true")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	if res.StatusCode != 200 {
		return "", fmt.Errorf("status code %d trying to GET %s", res.StatusCode, url)
	}

	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("error while reading response from azure metadata endpoint: %s", err)
	}

	return string(all), nil
}

// GetHostname returns hostname based on Azure instance metadata.
func GetHostname(ctx context.Context, options map[string]interface{}) (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}

	return getHostnameWithConfig(ctx, config.Datadog)
}

func getHostnameWithConfig(ctx context.Context, config config.Config) (string, error) {
	style := config.GetString(hostnameStyleSetting)

	if style == "os" {
		return "", fmt.Errorf("azure_hostname_style is set to 'os'")
	}

	metadataJSON, err := getResponse(ctx, metadataURL+"/metadata/instance/compute?api-version=2017-08-01")
	if err != nil {
		return "", fmt.Errorf("failed to get Azure instance metadata: %s", err)
	}

	var metadata struct {
		VMID              string
		Name              string
		ResourceGroupName string
		SubscriptionID    string
	}
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return "", fmt.Errorf("failed to parse Azure instance metadata: %s", err)
	}

	var name string
	switch style {
	case "vmid":
		name = metadata.VMID
	case "name":
		name = metadata.Name
	case "name_and_resource_group":
		name = fmt.Sprintf("%s.%s", metadata.Name, metadata.ResourceGroupName)
	case "full":
		name = fmt.Sprintf("%s.%s.%s", metadata.Name, metadata.ResourceGroupName, metadata.SubscriptionID)
	default:
		return "", fmt.Errorf("invalid azure_hostname_style value: %s", style)
	}

	if err := validate.ValidHostname(name); err != nil {
		return "", err
	}

	return name, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package azure

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
)

// declare these as vars not const to ease testing
var (
	metadataURL = "http://169.254.169.254"
	timeout     = 300 * time.Millisecond

	// CloudProviderName contains the inventory name of for EC2
	CloudProviderName = "Azure"
)

// GetHostAlias returns the VM ID from the Azure Metadata api
func GetHostAlias() (string, error) {
	res, err := getResponseWithMaxLength(metadataURL+"/metadata/instance/compute/vmId?api-version=2017-04-02&format=text",
		config.Datadog.GetInt("metadata_endpoints_max_hostname_size"))
	if err != nil {
		return "", fmt.Errorf("Azure HostAliases: unable to query metadata endpoint: %s", err)
	}

	// registering that we're running on Azure
	inventories.SetAgentMetadata(inventories.CloudProviderMetatadaName, CloudProviderName)
	return res, nil
}

// GetClusterName returns the name of the cluster containing the current VM
func GetClusterName() (string, error) {
	all, err := getResponse(metadataURL + "/metadata/instance/compute/resourceGroupName?api-version=2017-08-01&format=text")
	if err != nil {
		return "", fmt.Errorf("unable to query metadata endpoint: %s", err)
	}

	splitAll := strings.Split(string(all), "_")
	if len(splitAll) < 4 || splitAll[0] != "MC" {
		return "", errors.New("cannot parse the clustername from metadata")
	}

	clusterName := splitAll[len(splitAll)-2]
	return clusterName, nil
}

func getResponseWithMaxLength(endpoint string, maxLength int) (string, error) {
	result, err := getResponse(endpoint)
	if err != nil {
		return result, err
	}
	if len(result) > maxLength {
		return "", fmt.Errorf("%v gave a response with length > to %v", endpoint, maxLength)
	}
	return result, err
}

func getResponse(url string) (string, error) {
	client := http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequest("GET", url, nil)
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

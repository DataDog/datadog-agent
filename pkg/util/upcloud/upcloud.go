// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upcloud

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// declare these as vars not const to ease testing
var (
	metadataURL = "http://169.254.169.254"
	timeout     = 300 * time.Millisecond

	// CloudProviderName contains the inventory name of UpCloud
	CloudProviderName = "UpCloud"
)

type Metadata struct {
	CloudName  string `json:"cloud_name"`
	InstanceId string `json:"instance_id"`
	Network    Network
}

type Network struct {
	Interfaces []Interface
}

type Interface struct {
	Index       uint8
	IpAddresses []IpAddress `json:"ip_addresses"`
	NetworkId   string      `json:"network_id"`
	Type        string
}

type IpAddress struct {
	Address string
	Family  string
}

// IsRunningOn returns true if the agent is running on UpCloud
func IsRunningOn() bool {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return false
	}
	cloudName, err := getResponse(metadataURL + "/metadata/v1/cloud_name")
	if err != nil || string(cloudName) != "upcloud" {
		return false
	}
	return true
}

// GetHostAlias returns the VM UUID from the UpCloud metadata API
func GetHostAlias() (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}
	res, err := getResponseWithMaxLength(metadataURL+"/metadata/v1/instance_id",
		config.Datadog.GetInt("metadata_endpoints_max_hostname_size"))
	if err != nil {
		return "", fmt.Errorf("UpCloud GetHostAlias: unable to query metadata endpoint: %s", err)
	}
	return string(res), err
}

// GetPublicIPv4 returns the public IPv4 address of the current UpCloud instance
func GetPublicIPv4() (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}
	result, err := getResponse(metadataURL + "/metadata/v1.json")
	if err != nil {
		return "", err
	}
	var metadata Metadata
	err = json.Unmarshal(result, &metadata)
	if err != nil {
		return "", err
	}
	for _, iface := range metadata.Network.Interfaces {
		if iface.Type != "public" {
			continue
		}
		for _, ip := range iface.IpAddresses {
			if ip.Family == "IPv4" {
				return ip.Address, nil
			}
		}
	}
	return "", nil
}

// GetNetworkID returns the network ID of the current UpCloud instance.
// For UpCloud instances, the network ID is non-empty, if the instance is found to be part of exactly one network.
func GetNetworkID() (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}
	result, err := getResponse(metadataURL + "/metadata/v1.json")
	if err != nil {
		return "", err
	}
	var metadata Metadata
	err = json.Unmarshal(result, &metadata)
	if err != nil {
		return "", err
	}
	if len(metadata.Network.Interfaces) == 0 {
		return "", fmt.Errorf("zero network interfaces detected")
	}
	var id string
	for _, iface := range metadata.Network.Interfaces {
		if id != "" && id != iface.NetworkId {
			return "", fmt.Errorf("interfaces in more than one network detected, cannot get network ID")
		}
		id = iface.NetworkId
	}
	return id, nil
}

// GetNTPHosts returns nil for UpCloud.
func GetNTPHosts() []string {

	return nil
}

func getResponseWithMaxLength(endpoint string, maxLength int) ([]byte, error) {
	result, err := getResponse(endpoint)
	if err != nil {
		return result, err
	}
	if len(result) > maxLength {
		return nil, fmt.Errorf("%v gave a response with length > to %v", endpoint, maxLength)
	}
	return result, err
}

func getResponse(url string) ([]byte, error) {
	client := http.Client{
		Transport: httputils.CreateHTTPTransport(),
		Timeout:   timeout,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code %d trying to GET %s", res.StatusCode, url)
	}

	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error while reading response from UpCloud metadata endpoint: %s", err)
	}

	return all, nil
}

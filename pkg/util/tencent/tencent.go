// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package tencent

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// declare these as vars not const to ease testing
var (
	metadataURL = "http://169.254.0.23"
	timeout     = 300 * time.Millisecond

	// CloudProviderName contains the inventory name of for CVM
	CloudProviderName = "Tencent"
)

// IsRunningOn returns true if the agent is running on Tencent Cloud
func IsRunningOn() bool {
	if _, err := GetInstanceID(); err == nil {
		return true
	}
	return false
}

// GetHostAlias returns the VM ID from the Tencent Metadata api
func GetHostAlias() (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}
	return GetInstanceID()
}

// GetInstanceID fetches the instance id for current host from the Tencent metadata API
func GetInstanceID() (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}
	res, err := getMetadataItemWithMaxLength(metadataURL+"/meta-data/instance-id", config.Datadog.GetInt("metadata_endpoints_max_hostname_size"))
	if err != nil {
		return "", fmt.Errorf("unable to get TencentCloud CVM instanceID: %s", err)
	}
	return res, err
}

// HostnameProvider gets the hostname
func HostnameProvider() (string, error) {
	log.Debug("GetHostname trying Tencent metadata...")
	return GetInstanceID()
}

func getMetadataItemWithMaxLength(endpoint string, maxLength int) (string, error) {
	result, err := getMetadataItem(endpoint)
	if err != nil {
		return result, err
	}
	if len(result) > maxLength {
		return "", fmt.Errorf("%v gave a response with length > to %v", endpoint, maxLength)
	}
	return result, err
}

func getMetadataItem(endpoint string) (string, error) {
	res, err := getResponse(endpoint)
	if err != nil {
		return "", fmt.Errorf("unable to fetch Tencent Metadata API, %s", err)
	}

	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read response body, %s", err)
	}

	return string(all), nil
}

func getResponse(url string) (*http.Response, error) {
	client := http.Client{
		Timeout: timeout,
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
		return nil, fmt.Errorf("status code %d trying to fetch %s", res.StatusCode, url)
	}

	return res, nil
}

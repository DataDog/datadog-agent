// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package ec2

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// declare these as vars not const to ease testing
var (
	metadataURL        = "http://169.254.169.254/latest/meta-data"
	tokenURL           = "http://169.254.169.254/latest/api/token"
	timeout            = 100 * time.Millisecond
	oldDefaultPrefixes = []string{"ip-", "domu"}
	defaultPrefixes    = []string{"ip-", "domu", "ec2amaz-"}

	// CloudProviderName contains the inventory name of for EC2
	CloudProviderName = "AWS"
)

// GetInstanceID fetches the instance id for current host from the EC2 metadata API
func GetInstanceID() (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}
	return getMetadataItemWithMaxLength("/instance-id", config.Datadog.GetInt("metadata_endpoints_max_hostname_size"))
}

// GetLocalIPv4 gets the local IPv4 for the currently running host using the EC2 metadata API.
// Returns a []string to implement the HostIPProvider interface expected in pkg/process/util
func GetLocalIPv4() ([]string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return nil, fmt.Errorf("cloud provider is disabled by configuration")
	}
	ip, err := getMetadataItem("/local-ipv4")
	if err != nil {
		return nil, err
	}
	return []string{ip}, nil
}

// IsRunningOn returns true if the agent is running on AWS
func IsRunningOn() bool {
	if _, err := GetHostname(); err == nil {
		return true
	}
	return false
}

// GetHostname fetches the hostname for current host from the EC2 metadata API
func GetHostname() (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}
	return getMetadataItemWithMaxLength("/hostname", config.Datadog.GetInt("metadata_endpoints_max_hostname_size"))
}

// GetNetworkID retrieves the network ID using the EC2 metadata endpoint. For
// EC2 instances, the the network ID is the VPC ID, if the instance is found to
// be a part of exactly one VPC.
func GetNetworkID() (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}
	resp, err := getMetadataItem("/network/interfaces/macs")
	if err != nil {
		return "", err
	}

	macs := strings.Split(strings.TrimSpace(resp), "\n")
	vpcIDs := common.NewStringSet()

	for _, mac := range macs {
		if mac == "" {
			continue
		}
		mac = strings.TrimSuffix(mac, "/")
		id, err := getMetadataItem(fmt.Sprintf("/network/interfaces/macs/%s/vpc-id", mac))
		if err != nil {
			return "", err
		}
		vpcIDs.Add(id)
	}

	switch len(vpcIDs) {
	case 0:
		return "", fmt.Errorf("EC2: GetNetworkID no mac addresses returned")
	case 1:
		return vpcIDs.GetAll()[0], nil
	default:
		return "", fmt.Errorf("EC2: GetNetworkID too many mac addresses returned")
	}
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
	res, err := doHTTPRequest(metadataURL+endpoint, http.MethodGet, map[string]string{}, true)
	if err != nil {
		return "", fmt.Errorf("unable to fetch EC2 API, %s", err)
	}

	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read response body, %s", err)
	}

	return string(all), nil
}

// GetClusterName returns the name of the cluster containing the current EC2 instance
func GetClusterName() (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}
	tags, err := GetTags()
	if err != nil {
		return "", fmt.Errorf("unable to retrieve clustername from EC2: %s", err)
	}

	return extractClusterName(tags)
}

func extractClusterName(tags []string) (string, error) {
	var clusterName string
	for _, tag := range tags {
		if strings.HasPrefix(tag, "kubernetes.io/cluster/") { // tag key format: kubernetes.io/cluster/clustername"
			key := strings.Split(tag, ":")[0]
			clusterName = strings.Split(key, "/")[2] // rely on ec2 tag format to extract clustername
			break
		}
	}

	if clusterName == "" {
		return "", errors.New("unable to parse cluster name from EC2 tags")
	}

	return clusterName, nil
}

func doHTTPRequest(url string, method string, headers map[string]string, retriableWithFreshToken bool) (*http.Response, error) {
	client := http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	for header, value := range headers {
		req.Header.Add(header, value)
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == 401 && retriableWithFreshToken {
		// Most of 401 errors can be solved by retrying with a fresh token
		token, err := getToken()
		if err != nil {
			return nil, err
		}
		headers["X-aws-ec2-metadata-token"] = token
		return doHTTPRequest(url, method, headers, false)

	} else if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code %d trying to fetch %s", res.StatusCode, url)
	}

	return res, nil
}

func getToken() (string, error) {
	client := http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequest(http.MethodPut, tokenURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Add("X-aws-ec2-metadata-token-ttl-seconds", "60")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	if res.StatusCode != 200 {
		return "", fmt.Errorf("status code %d trying to fetch %s", res.StatusCode, tokenURL)
	}

	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read response body, %s", err)
	}

	return string(all), nil
}

// IsDefaultHostname returns whether the given hostname is a default one for EC2
func IsDefaultHostname(hostname string) bool {
	return isDefaultHostname(hostname, config.Datadog.GetBool("ec2_use_windows_prefix_detection"))
}

// IsDefaultHostnameForIntake returns whether the given hostname is a default one for EC2 for the intake
func IsDefaultHostnameForIntake(hostname string) bool {
	return isDefaultHostname(hostname, false)
}

// IsWindowsDefaultHostname returns whether the given hostname is a Windows default one for EC2 (starts with 'ec2amaz-')
func IsWindowsDefaultHostname(hostname string) bool {
	return !isDefaultHostname(hostname, false) && isDefaultHostname(hostname, true)
}

func isDefaultHostname(hostname string, useWindowsPrefix bool) bool {
	hostname = strings.ToLower(hostname)
	isDefault := false

	var prefixes []string

	if useWindowsPrefix {
		prefixes = defaultPrefixes
	} else {
		prefixes = oldDefaultPrefixes
	}

	for _, val := range prefixes {
		isDefault = isDefault || strings.HasPrefix(hostname, val)
	}
	return isDefault
}

// HostnameProvider gets the hostname
func HostnameProvider() (string, error) {
	log.Debug("GetHostname trying EC2 metadata...")
	return GetInstanceID()
}

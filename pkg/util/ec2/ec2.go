// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package ec2

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/ecs"
	log "github.com/cihub/seelog"
)

// declare these as vars not const to ease testing
var (
	metadataURL         = "http://169.254.169.254/latest/meta-data"
	instanceIdentityURL = "http://169.254.169.254/latest/dynamic/instance-identity/document/"
	timeout             = 100 * time.Millisecond
	defaultPrefixes     = []string{"ip-", "domu"}
)

// GetInstanceID fetches the instance id for current host from the EC2 metadata API
func GetInstanceID() (string, error) {
	return getMetadataItem("/instance-id")
}

// GetHostname fetches the hostname for current host from the EC2 metadata API
func GetHostname() (string, error) {
	return getMetadataItem("/hostname")
}

func getMetadataItem(endpoint string) (string, error) {
	res, err := getResponse(metadataURL + endpoint)
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

// IsDefaultHostname returns whether the given hostname is a default one for EC2
func IsDefaultHostname(hostname string) bool {
	hostname = strings.ToLower(hostname)
	isDefault := false
	for _, val := range defaultPrefixes {
		isDefault = isDefault || strings.HasPrefix(hostname, val)
	}
	return isDefault
}

// HostnameProvider gets the hostname
func HostnameProvider(hostName string) (string, error) {
	if ecs.IsInstance() || IsDefaultHostname(hostName) {
		log.Debug("GetHostname trying EC2 metadata...")
		return GetInstanceID()
	}
	return "", nil
}

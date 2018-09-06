// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package alibaba

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

// declare these as vars not const to ease testing
var (
	metadataURL = "http://100.100.100.200"
	timeout     = 300 * time.Millisecond
)

// GetHostAlias returns the VM ID from the Alibaba Metadata api
func GetHostAlias() (string, error) {
	res, err := getResponse(metadataURL + "/latest/meta-data/instance-id")
	if err != nil {
		return "", fmt.Errorf("Alibaba HostAliases: unable to query metadata endpoint: %s", err)
	}

	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("error while reading response from alibaba metadata endpoint: %s", err)
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
		return nil, fmt.Errorf("status code %d trying to GET %s", res.StatusCode, url)
	}

	return res, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tencent

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cachedfetch"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// declare these as vars not const to ease testing
var (
	metadataURL = "http://169.254.0.23"
	timeout     = 300 * time.Millisecond

	// CloudProviderName contains the inventory name of for CVM
	CloudProviderName = "Tencent"
)

// IsRunningOn returns true if the agent is running on Tencent Cloud
func IsRunningOn(ctx context.Context) bool {
	if _, err := GetInstanceID(ctx); err == nil {
		return true
	}
	return false
}

// GetHostAliases returns the VM ID from the Tencent Metadata api
func GetHostAliases(ctx context.Context) ([]string, error) {
	alias, err := GetInstanceID(ctx)
	if err == nil {
		return []string{alias}, nil
	}
	return nil, err
}

var instanceIDFetcher = cachedfetch.Fetcher{
	Name: "Tencent InstanceID",
	Attempt: func(ctx context.Context) (interface{}, error) {
		res, err := getMetadataItemWithMaxLength(ctx, metadataURL+"/meta-data/instance-id", config.Datadog.GetInt("metadata_endpoints_max_hostname_size"))
		if err != nil {
			return "", fmt.Errorf("unable to get TencentCloud CVM instanceID: %s", err)
		}
		return res, err
	},
}

// GetInstanceID fetches the instance id for current host from the Tencent metadata API
func GetInstanceID(ctx context.Context) (string, error) {
	return instanceIDFetcher.FetchString(ctx)
}

// GetNTPHosts returns the NTP hosts for Tencent if it is detected as the cloud provider, otherwise an empty array.
// Demo: https://intl.cloud.tencent.com/document/product/213/32379
func GetNTPHosts(ctx context.Context) []string {
	if IsRunningOn(ctx) {
		return []string{"ntpupdate.tencentyun.com"}
	}

	return nil
}

func getMetadataItemWithMaxLength(ctx context.Context, endpoint string, maxLength int) (string, error) {
	result, err := getMetadataItem(ctx, endpoint)
	if err != nil {
		return result, err
	}
	if len(result) > maxLength {
		return "", fmt.Errorf("%v gave a response with length > to %v", endpoint, maxLength)
	}
	return result, err
}

func getMetadataItem(ctx context.Context, endpoint string) (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}

	res, err := httputils.Get(ctx, endpoint, nil, timeout)
	if err != nil {
		return "", fmt.Errorf("unable to fetch Tencent Metadata API, %s", err)
	}
	return res, nil
}

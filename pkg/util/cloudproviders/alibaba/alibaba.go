// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package alibaba provides utilities to detect the Alibaba cloud provider.
package alibaba

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
	metadataURL = "http://100.100.100.200"
	timeout     = 300 * time.Millisecond

	// CloudProviderName contains the inventory name of for EC2
	CloudProviderName = "Alibaba"
)

// IsRunningOn returns true if the agent is running on Alibaba
func IsRunningOn(ctx context.Context) bool {
	if _, err := GetHostAliases(ctx); err == nil {
		return true
	}
	return false
}

var instanceIDFetcher = cachedfetch.Fetcher{
	Name: "Alibaba InstanceID",
	Attempt: func(ctx context.Context) (interface{}, error) {
		if !config.IsCloudProviderEnabled(CloudProviderName) {
			return nil, fmt.Errorf("cloud provider is disabled by configuration")
		}

		endpoint := metadataURL + "/latest/meta-data/instance-id"
		res, err := httputils.Get(ctx, endpoint, nil, timeout, config.Datadog)
		if err != nil {
			return nil, fmt.Errorf("Alibaba HostAliases: unable to query metadata endpoint: %s", err)
		}
		maxLength := config.Datadog.GetInt("metadata_endpoints_max_hostname_size")
		if len(res) > maxLength {
			return nil, fmt.Errorf("%v gave a response with length > to %v", endpoint, maxLength)
		}
		return []string{res}, nil
	},
}

// GetHostAliases returns the VM ID from the Alibaba Metadata api
func GetHostAliases(ctx context.Context) ([]string, error) {
	return instanceIDFetcher.FetchStringSlice(ctx)
}

// GetNTPHosts returns the NTP hosts for Alibaba if it is detected as the cloud provider, otherwise an empty array.
// These are their public NTP servers, as Alibaba uses two different types of private/internal networks for their cloud
// machines and we can't be sure those servers are always accessible for every customer on every network type.
// Docs: https://www.alibabacloud.com/help/doc-detail/92704.htm
func GetNTPHosts(ctx context.Context) []string {
	if IsRunningOn(ctx) {
		return []string{
			"ntp.aliyun.com", "ntp1.aliyun.com", "ntp2.aliyun.com", "ntp3.aliyun.com",
			"ntp4.aliyun.com", "ntp5.aliyun.com", "ntp6.aliyun.com", "ntp7.aliyun.com",
		}
	}

	return nil
}

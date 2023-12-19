// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

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
	metadataURL = "http://169.254.169.254"
	timeout     = 300 * time.Millisecond

	// CloudProviderName contains the inventory name of for EC2
	CloudProviderName = "Oracle"
)

// IsRunningOn returns true if the agent is running on Oracle
func IsRunningOn(ctx context.Context) bool {
	if _, err := GetHostAliases(ctx); err == nil {
		return true
	}
	return false
}

var instanceIDFetcher = cachedfetch.Fetcher{
	Name: "Oracle InstanceID",
	Attempt: func(ctx context.Context) (interface{}, error) {
		if !config.IsCloudProviderEnabled(CloudProviderName) {
			return "", fmt.Errorf("Oracle cloud provider is disabled by configuration")
		}

		endpoint := metadataURL + "/opc/v2/instance/id"
		res, err := httputils.Get(ctx, endpoint, map[string]string{"Authorization": "Bearer Oracle"}, timeout, config.Datadog)
		if err != nil {
			return nil, fmt.Errorf("Oracle HostAliases: unable to query metadata endpoint: %s", err)
		}

		if res == "" {
			return nil, fmt.Errorf("Oracle '%s' returned empty id", endpoint)
		}

		maxLength := config.Datadog.GetInt("metadata_endpoints_max_hostname_size")
		if len(res) > maxLength {
			return nil, fmt.Errorf("%v gave a response with length > to %v", endpoint, maxLength)
		}
		return []string{res}, nil
	},
}

// GetHostAliases returns the VM ID from the Oracle Metadata api
func GetHostAliases(ctx context.Context) ([]string, error) {
	return instanceIDFetcher.FetchStringSlice(ctx)
}

// GetNTPHosts returns the NTP hosts for Oracle if it is detected as the cloud provider, otherwise an nil array.
// Docs: https://docs.oracle.com/en-us/iaas/Content/Compute/Tasks/configuringntpservice.htm
func GetNTPHosts(ctx context.Context) []string {
	if IsRunningOn(ctx) {
		return []string{"169.254.169.254"}
	}

	return nil
}

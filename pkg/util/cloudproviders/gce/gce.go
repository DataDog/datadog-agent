// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gce

import (
	"context"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cachedfetch"
	"github.com/DataDog/datadog-agent/pkg/util/common"
)

// declare these as vars not const to ease testing
var (
	metadataURL = "http://169.254.169.254/computeMetadata/v1"

	// CloudProviderName contains the inventory name of the cloud
	CloudProviderName = "GCP"
)

// IsRunningOn returns true if the agent is running on GCE
func IsRunningOn(ctx context.Context) bool {
	panic("not called")
}

var hostnameFetcher = cachedfetch.Fetcher{
	Name: "GCP Hostname",
	Attempt: func(ctx context.Context) (interface{}, error) {
		hostname, err := getResponseWithMaxLength(ctx, metadataURL+"/instance/hostname",
			config.Datadog.GetInt("metadata_endpoints_max_hostname_size"))
		if err != nil {
			return "", fmt.Errorf("unable to retrieve hostname from GCE: %s", err)
		}
		return hostname, nil
	},
}

// GetHostname returns the hostname querying GCE Metadata api
func GetHostname(ctx context.Context) (string, error) {
	panic("not called")
}

// GetHostAliases returns the host aliases from GCE
func GetHostAliases(ctx context.Context) ([]string, error) {
	panic("not called")
}

var nameFetcher = cachedfetch.Fetcher{
	Name: "GCP Instance Name",
	Attempt: func(ctx context.Context) (interface{}, error) {
		return getResponseWithMaxLength(ctx,
			metadataURL+"/instance/name",
			config.Datadog.GetInt("metadata_endpoints_max_hostname_size"))
	},
}

var projectIDFetcher = cachedfetch.Fetcher{
	Name: "GCP Project ID",
	Attempt: func(ctx context.Context) (interface{}, error) {
		projectID, err := getResponseWithMaxLength(ctx,
			metadataURL+"/project/project-id",
			config.Datadog.GetInt("metadata_endpoints_max_hostname_size"))
		if err != nil {
			return "", fmt.Errorf("unable to retrieve project ID from GCE: %s", err)
		}
		return projectID, err
	},
}

// GetProjectID returns the project ID of the current GCE instance
func GetProjectID(ctx context.Context) (string, error) {
	panic("not called")
}

func getInstanceAlias(ctx context.Context, hostname string) (string, error) {
	panic("not called")
}

var clusterNameFetcher = cachedfetch.Fetcher{
	Name: "GCP Cluster Name",
	Attempt: func(ctx context.Context) (interface{}, error) {
		clusterName, err := getResponseWithMaxLength(ctx, metadataURL+"/instance/attributes/cluster-name",
			config.Datadog.GetInt("metadata_endpoints_max_hostname_size"))
		if err != nil {
			return "", fmt.Errorf("unable to retrieve clustername from GCE: %s", err)
		}
		return clusterName, nil
	},
}

// GetClusterName returns the name of the cluster containing the current GCE instance
func GetClusterName(ctx context.Context) (string, error) {
	panic("not called")
}

var publicIPv4Fetcher = cachedfetch.Fetcher{
	Name: "GCP Public IP",
	Attempt: func(ctx context.Context) (interface{}, error) {
		publicIPv4, err := getResponseWithMaxLength(ctx, metadataURL+"/instance/network-interfaces/0/access-configs/0/external-ip",
			config.Datadog.GetInt("metadata_endpoints_max_hostname_size"))
		if err != nil {
			return "", fmt.Errorf("unable to retrieve public IPv4 from GCE: %s", err)
		}
		return publicIPv4, nil
	},
}

// GetPublicIPv4 returns the public IPv4 address of the current GCE instance
func GetPublicIPv4(ctx context.Context) (string, error) {
	panic("not called")
}

var networkIDFetcher = cachedfetch.Fetcher{
	Name: "GCP Network ID",
	Attempt: func(ctx context.Context) (interface{}, error) {
		resp, err := getResponse(ctx, metadataURL+"/instance/network-interfaces/")
		if err != nil {
			return "", fmt.Errorf("unable to retrieve network-interfaces from GCE: %s", err)
		}

		interfaceIDs := strings.Split(strings.TrimSpace(resp), "\n")
		vpcIDs := common.NewStringSet()

		for _, interfaceID := range interfaceIDs {
			if interfaceID == "" {
				continue
			}
			interfaceID = strings.TrimSuffix(interfaceID, "/")
			id, err := getResponse(ctx, metadataURL+fmt.Sprintf("/instance/network-interfaces/%s/network", interfaceID))
			if err != nil {
				return "", err
			}
			vpcIDs.Add(id)
		}

		switch len(vpcIDs) {
		case 0:
			return "", fmt.Errorf("zero network interfaces detected")
		case 1:
			return vpcIDs.GetAll()[0], nil
		default:
			return "", fmt.Errorf("more than one network interface detected, cannot get network ID")
		}
	},
}

// GetNetworkID retrieves the network ID using the metadata endpoint. For
// GCE instances, the the network ID is the VPC ID, if the instance is found to
// be a part of exactly one VPC.
func GetNetworkID(ctx context.Context) (string, error) {
	panic("not called")
}

// GetNTPHosts returns the NTP hosts for GCE if it is detected as the cloud provider, otherwise an empty array.
// Docs: https://cloud.google.com/compute/docs/instances/managing-instances
func GetNTPHosts(ctx context.Context) []string {
	panic("not called")
}

func getResponseWithMaxLength(ctx context.Context, endpoint string, maxLength int) (string, error) {
	panic("not called")
}

func getResponse(ctx context.Context, url string) (string, error) {
	panic("not called")
}

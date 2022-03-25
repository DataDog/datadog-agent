// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gce

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cachedfetch"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// declare these as vars not const to ease testing
var (
	metadataURL = "http://169.254.169.254/computeMetadata/v1"

	// CloudProviderName contains the inventory name of the cloud
	CloudProviderName = "GCP"
)

// IsRunningOn returns true if the agent is running on GCE
func IsRunningOn(ctx context.Context) bool {
	if _, err := GetHostname(ctx); err == nil {
		return true
	}
	return false
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
	return hostnameFetcher.FetchString(ctx)
}

// GetHostAliases returns the host aliases from GCE
func GetHostAliases(ctx context.Context) ([]string, error) {
	aliases := []string{}

	hostname, err := GetHostname(ctx)
	if err == nil {
		aliases = append(aliases, hostname)
	} else {
		log.Debugf("failed to get hostname to use as Host Alias: %s", err)
	}

	if instanceAlias, err := getInstanceAlias(ctx, hostname); err == nil {
		aliases = append(aliases, instanceAlias)
	} else {
		log.Debugf("failed to get Host Alias: %s", err)
	}

	return aliases, nil
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

func getInstanceAlias(ctx context.Context, hostname string) (string, error) {
	instanceName, err := nameFetcher.FetchString(ctx)
	if err != nil {
		// If the endpoint is not reachable, fallback on the old way to get the alias.
		// For instance, it happens in GKE, where the metadata server is only a subset
		// of the Compute Engine metadata server.
		// See https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity#gke_mds
		if hostname == "" {
			return "", fmt.Errorf("unable to retrieve instance name and hostname from GCE: %s", err)
		}
		instanceName = strings.SplitN(hostname, ".", 2)[0]
	}

	projectID, err := projectIDFetcher.FetchString(ctx)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s.%s", instanceName, projectID), nil
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
	return clusterNameFetcher.FetchString(ctx)
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
	return publicIPv4Fetcher.FetchString(ctx)
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
	return networkIDFetcher.FetchString(ctx)
}

// GetNTPHosts returns the NTP hosts for GCE if it is detected as the cloud provider, otherwise an empty array.
// Docs: https://cloud.google.com/compute/docs/instances/managing-instances
func GetNTPHosts(ctx context.Context) []string {
	if IsRunningOn(ctx) {
		return []string{"metadata.google.internal"}
	}

	return nil
}

func getResponseWithMaxLength(ctx context.Context, endpoint string, maxLength int) (string, error) {
	result, err := getResponse(ctx, endpoint)
	if err != nil {
		return result, err
	}
	if len(result) > maxLength {
		return "", fmt.Errorf("%v gave a response with length > to %v", endpoint, maxLength)
	}
	return result, err
}

func getResponse(ctx context.Context, url string) (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}

	res, err := httputils.Get(ctx, url, map[string]string{"Metadata-Flavor": "Google"}, config.Datadog.GetDuration("gce_metadata_timeout")*time.Millisecond)
	if err != nil {
		return "", fmt.Errorf("GCE metadata API error: %s", err)
	}

	// Some cloud platforms will respond with an empty body, causing the agent to assume a faulty hostname
	if len(res) <= 0 {
		return "", fmt.Errorf("empty response body")
	}

	return res, nil
}

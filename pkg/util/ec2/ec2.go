// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/util/cachedfetch"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// declare these as vars not const to ease testing
var (
	metadataURL        = "http://169.254.169.254/latest/meta-data"
	tokenURL           = "http://169.254.169.254/latest/api/token"
	oldDefaultPrefixes = []string{"ip-", "domu"}
	defaultPrefixes    = []string{"ip-", "domu", "ec2amaz-"}

	token              *httputils.APIToken
	tokenRenewalWindow = 15 * time.Second

	// CloudProviderName contains the inventory name of for EC2
	CloudProviderName = "AWS"
	// DMIBoardVendor contains the DMI board vendor for EC2
	DMIBoardVendor = "Amazon EC2"

	currentMetadataSource = metadataSourceNone
)

func init() {
	token = httputils.NewAPIToken(getToken)
}

const (
	metadataSourceNone = iota
	metadataSourceUUID
	metadataSourceDMI
	metadataSourceIMDSv1
	metadataSourceIMDSv2
)

// setCloudProviderSource set the best source available for EC2 metadata to the inventories payload.
//
// The different sources that can be used to know if we are running on EC2. This data is registered in the
// inventories metadata payload.
//
// We current have 3 ways to know we're on EC2 (if one fails we fallback to the next):
// - we succeed in reaching IMDS v1 or v2 metadata API.
// - the DMI information match EC2 and we can get the instanceID from it.
// - the product UUID or hypervisor UUID match EC2 (we know we're on EC2 but can't fetch the instance ID).
//
// Since some ways can temporary fail, we always register the "best" that worked at some point. This is mainly aimed at
// IMDS which is sometimes unavailable at startup.
func setCloudProviderSource(source int) {
	if source <= currentMetadataSource {
		return
	}

	sourceName := ""
	currentMetadataSource = source

	switch source {
	case metadataSourceUUID:
		sourceName = "UUID"
	case metadataSourceDMI:
		sourceName = "DMI"
	case metadataSourceIMDSv1:
		sourceName = "IMDSv1"
	case metadataSourceIMDSv2:
		sourceName = "IMDSv2"
	default: // unknown source or metadataSourceNone
		return
	}

	inventories.SetHostMetadata(inventories.HostCloudProviderSource, sourceName)
}

var instanceIDFetcher = cachedfetch.Fetcher{
	Name: "EC2 InstanceID",
	Attempt: func(ctx context.Context) (interface{}, error) {
		return getMetadataItemWithMaxLength(ctx, imdsInstanceID, false)
	},
}

// GetInstanceID fetches the instance id for current host from the EC2 metadata API
func GetInstanceID(ctx context.Context) (string, error) {
	return instanceIDFetcher.FetchString(ctx)
}

// IsRunningOn returns true if the agent is running on AWS
func IsRunningOn(ctx context.Context) bool {
	if _, err := GetHostname(ctx); err == nil {
		return true
	}
	if isBoardVendorEC2() || isEC2UUID() {
		return true
	}

	return config.IsFeaturePresent(config.ECSEC2) || config.IsFeaturePresent(config.ECSFargate)
}

// GetHostAliases returns the host aliases from the EC2 metadata API.
func GetHostAliases(ctx context.Context) ([]string, error) {
	// We leverage GetHostAliases to register the instance ID in inventories
	registerCloudProviderHostnameID(ctx)

	instanceID, err := GetInstanceID(ctx)
	if err == nil {
		return []string{instanceID}, nil
	}
	log.Debugf("failed to get instance ID from metadata API for Host Alias: %s", err)

	// we fallback on DMI
	instanceID, err = getInstanceIDFromDMI()
	if err == nil {
		return []string{instanceID}, nil
	}
	log.Debugf("failed to get instance ID from DMI for Host Alias: %s", err)

	return []string{}, nil
}

var hostnameFetcher = cachedfetch.Fetcher{
	Name: "EC2 Hostname",
	Attempt: func(ctx context.Context) (interface{}, error) {
		return getMetadataItemWithMaxLength(ctx, imdsHostname, false)
	},
}

// GetHostname fetches the hostname for current host from the EC2 metadata API
func GetHostname(ctx context.Context) (string, error) {
	return hostnameFetcher.FetchString(ctx)
}

// GetNTPHosts returns the NTP hosts for EC2 if it is detected as the cloud provider, otherwise an empty array.
// Docs: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/set-time.html#configure_ntp
func GetNTPHosts(ctx context.Context) []string {
	if IsRunningOn(ctx) {
		return []string{"169.254.169.123"}
	}

	return nil
}

// GetClusterName returns the name of the cluster containing the current EC2 instance
func GetClusterName(ctx context.Context) (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}
	tags, err := fetchTagsFromCache(ctx)
	if err != nil {
		return "", err
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

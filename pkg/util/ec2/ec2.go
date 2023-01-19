// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cachedfetch"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
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
)

func init() {
	token = httputils.NewAPIToken(getToken)
}

func isBoardVendorEc2() bool {
	if !config.Datadog.GetBool("ec2_use_dmi") {
		return false
	}

	vendor := dmi.GetBoardVendor()
	if vendor == DMIBoardVendor {
		return true
	}
	return false
}

// getInstanceIDFromDMI fetches the instance id for current host from DMI
//
// On AWS Nitro instances dmi information contains the instanceID for the host. We check that the board vendor is
// EC2 and that the board_asset_tag match an instanceID format before using it
func getInstanceIDFromDMI() (string, error) {
	if !config.Datadog.GetBool("ec2_use_dmi") {
		return "", fmt.Errorf("'ec2_use_dmi' is disabled")
	}

	if !isBoardVendorEc2() {
		return "", fmt.Errorf("board vendor is not AWS")
	}

	boardAssetTag := dmi.GetBoardAssetTag()
	if boardAssetTag == "" || !strings.HasPrefix(boardAssetTag, "i-") {
		return "", fmt.Errorf("invalid board_asset_tag: '%s'", boardAssetTag)
	}
	return boardAssetTag, nil
}

var instanceIDFetcher = cachedfetch.Fetcher{
	Name: "EC2 InstanceID",
	Attempt: func(ctx context.Context) (interface{}, error) {
		return getMetadataItemWithMaxLength(ctx,
			"/instance-id",
			config.Datadog.GetInt("metadata_endpoints_max_hostname_size"),
		)
	},
}

// GetInstanceID fetches the instance id for current host from the EC2 metadata API
func GetInstanceID(ctx context.Context) (string, error) {
	return instanceIDFetcher.FetchString(ctx)
}

var publicIPv4Fetcher = cachedfetch.Fetcher{
	Name: "EC2 Public IPv4 Address",
	Attempt: func(ctx context.Context) (interface{}, error) {
		return getMetadataItem(ctx, "/public-ipv4")
	},
}

// GetPublicIPv4 gets the public IPv4 for the currently running host using the EC2 metadata API.
func GetPublicIPv4(ctx context.Context) (string, error) {
	return publicIPv4Fetcher.FetchString(ctx)
}

// IsRunningOn returns true if the agent is running on AWS
func IsRunningOn(ctx context.Context) bool {
	if config.IsFeaturePresent(config.ECSEC2) || config.IsFeaturePresent(config.ECSFargate) {
		return true
	}

	if _, err := GetHostname(ctx); err == nil {
		return true
	}
	return isBoardVendorEc2()
}

// GetHostAliases returns the host aliases from the EC2 metadata API.
func GetHostAliases(ctx context.Context) ([]string, error) {
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
		return getMetadataItemWithMaxLength(ctx,
			"/hostname",
			config.Datadog.GetInt("metadata_endpoints_max_hostname_size"),
		)
	},
}

// GetHostname fetches the hostname for current host from the EC2 metadata API
func GetHostname(ctx context.Context) (string, error) {
	return hostnameFetcher.FetchString(ctx)
}

var networkIDFetcher = cachedfetch.Fetcher{
	Name: "VPC IDs",
	Attempt: func(ctx context.Context) (interface{}, error) {
		resp, err := getMetadataItem(ctx, "/network/interfaces/macs")
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
			id, err := getMetadataItem(ctx, fmt.Sprintf("/network/interfaces/macs/%s/vpc-id", mac))
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
	},
}

// GetNetworkID retrieves the network ID using the EC2 metadata endpoint. For
// EC2 instances, the the network ID is the VPC ID, if the instance is found to
// be a part of exactly one VPC.
func GetNetworkID(ctx context.Context) (string, error) {
	return networkIDFetcher.FetchString(ctx)
}

// Subnet stores information about an AWS subnet
type Subnet struct {
	ID   string
	Cidr string
}

// GetSubnetForHardwareAddr returns info about the subnet associated with a hardware
// address (mac address) on the current host
func GetSubnetForHardwareAddr(ctx context.Context, hwAddr net.HardwareAddr) (subnet Subnet, err error) {
	if len(hwAddr) == 0 {
		err = fmt.Errorf("could not get subnet for empty hw addr")
		return
	}

	var resp string
	resp, err = getMetadataItem(ctx, fmt.Sprintf("/network/interfaces/macs/%s/subnet-id", hwAddr))
	if err != nil {
		return
	}

	subnet.ID = strings.TrimSpace(resp)

	resp, err = getMetadataItem(ctx, fmt.Sprintf("/network/interfaces/macs/%s/subnet-ipv4-cidr-block", hwAddr))
	if err != nil {
		return
	}

	subnet.Cidr = strings.TrimSpace(resp)
	return
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

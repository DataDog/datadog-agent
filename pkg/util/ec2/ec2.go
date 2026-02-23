// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ec2 provides information when running in ec2
package ec2

import (
	"context"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/cachedfetch"
	ec2internal "github.com/DataDog/datadog-agent/pkg/util/ec2/internal"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// declare these as vars not const to ease testing
var (
	oldDefaultPrefixes = []string{"ip-", "domu"}
	defaultPrefixes    = []string{"ip-", "domu", "ec2amaz-"}

	imdsInstanceID = "/instance-id"
)

const (
	// CloudProviderName contains the inventory name of for EC2
	CloudProviderName = ec2internal.CloudProviderName
	// DMIBoardVendor contains the DMI board vendor for EC2
	DMIBoardVendor = ec2internal.DMIBoardVendor

	ec2IMDSv2TransitionPayloadConfigFlag = "ec2_imdsv2_transition_payload_enabled"
)

// GetSourceName returns the source used to pull information for EC2 (UUID, DMI, IMDSv1 or IMDSv2)
func GetSourceName() string {
	return ec2internal.GetSourceName()
}

var instanceIDFetcher = cachedfetch.Fetcher{
	Name: "EC2 or DMI InstanceID",
	Attempt: func(ctx context.Context) (interface{}, error) {
		hostname, err := ec2internal.GetMetadataItemWithMaxLength(ctx, imdsInstanceID, ec2internal.UseIMDSv2(), true)
		if err != nil {
			if pkgconfigsetup.Datadog().GetBool(ec2IMDSv2TransitionPayloadConfigFlag) {
				log.Debugf("Failed to get instance ID from IMDSv2 - ec2_imdsv2_transition_payload_enabled is set, falling back on DMI: %s", err.Error())
				return getInstanceIDFromDMI()
			}
		}
		return hostname, err
	},
}

var imdsv2InstanceIDFetcher = cachedfetch.Fetcher{
	Name: "EC2 IMDSv2 InstanceID",
	Attempt: func(ctx context.Context) (interface{}, error) {
		return ec2internal.GetMetadataItemWithMaxLength(ctx, imdsInstanceID, ec2internal.ImdsV2, true)
	},
}

var legacyInstanceIDFetcher = cachedfetch.Fetcher{
	Name: "EC2 no IMDSv2 no DMI InstanceID",
	Attempt: func(ctx context.Context) (interface{}, error) {
		return ec2internal.GetMetadataItemWithMaxLength(ctx, imdsInstanceID, ec2internal.ImdsV1, false)
	},
}

// GetInstanceID fetches the instance id for current host from the EC2 metadata API
func GetInstanceID(ctx context.Context) (string, error) {
	return instanceIDFetcher.FetchString(ctx)
}

// GetLegacyResolutionInstanceID fetches the instance id for current host from the EC2 metadata API without using IMDSv2 or DMI (ie: only IMDSv1)
func GetLegacyResolutionInstanceID(ctx context.Context) (string, error) {
	return legacyInstanceIDFetcher.FetchString(ctx)
}

// GetIDMSv2InstanceID fetches the instance id for current host from the IMDSv2 EC2 metadata API
func GetIDMSv2InstanceID(ctx context.Context) (string, error) {
	return imdsv2InstanceIDFetcher.FetchString(ctx)
}

// GetHostID returns the instanceID for the current EC2 host using IMDSv2 only.
func GetHostID(ctx context.Context) string {
	instanceID, err := ec2internal.GetMetadataItemWithMaxLength(ctx, imdsInstanceID, ec2internal.ImdsV2, true)
	log.Debugf("instanceID from IMDSv2 '%s' (error: %v)", instanceID, err)

	if err == nil {
		return instanceID
	}
	return ""
}

// IsRunningOn returns true if the agent is running on AWS
func IsRunningOn(ctx context.Context) bool {
	if _, err := GetHostname(ctx); err == nil {
		return true
	}
	if isBoardVendorEC2() || isEC2UUID() {
		return true
	}

	return env.IsFeaturePresent(env.ECSEC2) || env.IsFeaturePresent(env.ECSFargate)
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

	// Try to use IMSDv2 if GetInstanceID didn't try it already
	if ec2internal.UseIMDSv2() == ec2internal.ImdsV1 {
		imsdv2InstanceID, err := GetIDMSv2InstanceID(ctx)
		if err == nil {
			return []string{imsdv2InstanceID}, nil
		}

		log.Debugf("failed to get instance ID from IMDSV2 for Host Alias: %s", err)
	}

	return []string{}, nil
}

var hostnameFetcher = cachedfetch.Fetcher{
	Name: "EC2 Hostname",
	Attempt: func(ctx context.Context) (interface{}, error) {
		return ec2internal.GetMetadataItemWithMaxLength(ctx, imdsHostname, ec2internal.UseIMDSv2(), true)
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

var instanceTypeFetcher = cachedfetch.Fetcher{
	Name: "EC2 Instance Type",
	Attempt: func(ctx context.Context) (interface{}, error) {
		return ec2internal.GetMetadataItemWithMaxLength(ctx, "/instance-type", ec2internal.UseIMDSv2(), false)
	},
}

// GetInstanceType returns the AWS instance type as reported by EC2 IMDS.
func GetInstanceType(ctx context.Context) (string, error) {
	return instanceTypeFetcher.FetchString(ctx)
}

// IsDefaultHostname returns whether the given hostname is a default one for EC2
func IsDefaultHostname(hostname string) bool {
	return isDefaultHostname(hostname, pkgconfigsetup.Datadog().GetBool("ec2_use_windows_prefix_detection"))
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

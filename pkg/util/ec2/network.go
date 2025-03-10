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

	"github.com/DataDog/datadog-agent/pkg/util/cachedfetch"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	ddhttp "github.com/DataDog/datadog-agent/pkg/util/http"
)

var publicIPv4Fetcher = cachedfetch.Fetcher{
	Name: "EC2 Public IPv4 Address",
	Attempt: func(ctx context.Context) (interface{}, error) {
		return getMetadataItem(ctx, imdsIPv4, useIMDSv2(), true)
	},
}

// GetPublicIPv4 gets the public IPv4 for the currently running host using the EC2 metadata API.
func GetPublicIPv4(ctx context.Context) (string, error) {
	return publicIPv4Fetcher.FetchString(ctx)
}

var networkIDFetcher = cachedfetch.Fetcher{
	Name: "VPC IDs",
	Attempt: func(ctx context.Context) (interface{}, error) {
		resp, err := getMetadataItem(ctx, imdsNetworkMacs, imdsV2, true)
		if err != nil {
			return "", fmt.Errorf("EC2: GetNetworkID failed to get mac addresses: %w", err)
		}

		macs := strings.Split(strings.TrimSpace(resp), "\n")
		vpcIDs := common.NewStringSet()

		for _, mac := range macs {
			if mac == "" {
				continue
			}
			mac = strings.TrimSuffix(mac, "/")
			id, err := getMetadataItem(ctx, fmt.Sprintf("%s/%s/vpc-id", imdsNetworkMacs, mac), imdsV2, true)
			if err != nil {
				return "", fmt.Errorf("EC2: GetNetworkID failed to get vpc id for mac %s: %w", mac, err)
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
	resp, err = getMetadataItem(ctx, fmt.Sprintf("%s/%s/subnet-id", imdsNetworkMacs, hwAddr), imdsV2, true)
	if err != nil {
		return
	}

	subnet.ID = strings.TrimSpace(resp)

	resp, err = getMetadataItem(ctx, fmt.Sprintf("%s/%s/subnet-ipv4-cidr-block", imdsNetworkMacs, hwAddr), imdsV2, true)
	if err != nil {
		return
	}

	subnet.Cidr = strings.TrimSpace(resp)
	return
}

var vpcSubnetFetcher = cachedfetch.Fetcher{
	Name: "VPC subnets",
	Attempt: func(ctx context.Context) (interface{}, error) {
		resp, err := getMetadataItem(ctx, imdsNetworkMacs, imdsV2, true)
		if err != nil {
			return nil, fmt.Errorf("EC2: GetVPCSubnetsForHost failed to get mac addresses: %w", err)
		}

		macs := strings.Split(resp, "\n")
		allSubnets := common.NewStringSet()

		addMAC := func(mac string, endpoint string) error {
			mac = strings.TrimSuffix(mac, "/")
			cidrs, err := getMetadataItem(ctx, fmt.Sprintf("%s/%s/%s", imdsNetworkMacs, mac, endpoint), imdsV2, true)
			var sce *ddhttp.StatusCodeError
			// if the interface doesn't have CIDRs, e.g. it's ipv4 only, then it will 404.
			// treat that as an empty list of CIDRs.
			if errors.As(err, &sce) && sce.StatusCode == 404 {
				return nil
			}
			if err != nil {
				return fmt.Errorf("EC2: GetVPCSubnetsForHost failed to get CIDRs for mac %s: %w", mac, err)
			}
			cidrList := strings.Split(cidrs, "\n")
			for _, cidr := range cidrList {
				allSubnets.Add(cidr)
			}
			return nil
		}

		for _, mac := range macs {
			if mac == "" {
				continue
			}
			err = addMAC(mac, "vpc-ipv4-cidr-blocks")
			if err != nil {
				return nil, err
			}
			err = addMAC(mac, "vpc-ipv6-cidr-blocks")
			if err != nil {
				return nil, err
			}
		}

		return allSubnets.GetAll(), nil
	},
}

// GetVPCSubnetsForHost gets all the subnets in the VPCs this host has network interfaces for
func GetVPCSubnetsForHost(ctx context.Context) ([]string, error) {
	return vpcSubnetFetcher.FetchStringSlice(ctx)
}

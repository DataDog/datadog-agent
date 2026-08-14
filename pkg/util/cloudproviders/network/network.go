// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package network provides utilities around cloud provider networking.
package network

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	networkIDCacheKey         = "networkID"
	vpcSubnetsForHostCacheKey = "vpcSubnetsForHost"
)

// for testing
var (
	getGCENetworkID = gce.GetNetworkID
	getEC2NetworkID = ec2.GetNetworkID
)

// GetNetworkID retrieves the network_id which can be used to improve network
// connection resolution. This can be configured or detected.  The
// following sources will be queried:
// * configuration
// * GCE
// * EC2
func GetNetworkID(ctx context.Context) (string, error) {
	return cache.Get[string](
		networkIDCacheKey,
		func() (string, error) {
			var networkID string
			// the id from configuration
			if networkID = pkgconfigsetup.Datadog().GetString("network.id"); networkID != "" {
				log.Debugf("GetNetworkID: using configured network ID: %s", networkID)
				return networkID, nil
			}

			cfg := pkgconfigsetup.Datadog()
			var errs []error

			if configutils.IsCloudProviderEnabled(gce.CloudProviderName, cfg) {
				log.Debugf("GetNetworkID trying GCE")
				networkID, err := getGCENetworkID(ctx)
				if err == nil {
					log.Debugf("GetNetworkID: using network ID from GCE metadata: %s", networkID)
					return networkID, nil
				}
				errs = append(errs, err)
			}

			if configutils.IsCloudProviderEnabled(ec2.CloudProviderName, cfg) {
				log.Debugf("GetNetworkID trying EC2")
				networkID, err := getEC2NetworkID(ctx)
				if err == nil {
					log.Debugf("GetNetworkID: using network ID from EC2 metadata: %s", networkID)
					return networkID, nil
				}
				errs = append(errs, err)
			}

			if len(errs) == 0 {
				return "", errors.New("cloud provider metadata is disabled by configuration")
			}
			return "", fmt.Errorf("could not detect network ID: %w", errors.Join(errs...))
		})
}

func getVPCSubnetsForHostImpl(ctx context.Context) ([]string, error) {
	subnets, ec2err := ec2.GetVPCSubnetsForHost(ctx)
	if ec2err == nil {
		return subnets, nil
	}

	// TODO support GCE, azure

	return nil, fmt.Errorf("could not detect VPC subnets: %w", errors.Join(ec2err))
}

// use a global to allow easy mocking
var getVPCSubnetsForHost = getVPCSubnetsForHostImpl

// GetVPCSubnetsForHost gets all the subnets in the VPCs this host has network interfaces for
func GetVPCSubnetsForHost(ctx context.Context) ([]netip.Prefix, error) {
	return cache.GetWithExpiration[[]netip.Prefix](
		vpcSubnetsForHostCacheKey,
		func() ([]netip.Prefix, error) {
			subnets, err := getVPCSubnetsForHost(ctx)
			if err != nil {
				return nil, err
			}

			var parsedSubnets []netip.Prefix
			for _, subnet := range subnets {
				ipnet, err := netip.ParsePrefix(subnet)
				if err != nil {
					return nil, err
				}
				parsedSubnets = append(parsedSubnets, ipnet)
			}

			return parsedSubnets, nil
		}, 15*time.Minute)
}

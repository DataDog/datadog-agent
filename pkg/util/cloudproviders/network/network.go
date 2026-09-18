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
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/oracle"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	networkIDCacheKey         = "networkID"
	networkIDProviderCacheKey = "networkIDProvider"
	vpcSubnetsForHostCacheKey = "vpcSubnetsForHost"
)

// for testing
var (
	getGCENetworkID = gce.GetNetworkID
	getEC2NetworkID = ec2.GetNetworkID

	isRunningOnEC2    = ec2.IsRunningOn
	isRunningOnGCE    = gce.IsRunningOn
	isRunningOnAzure  = azure.IsRunningOn
	isRunningOnOracle = oracle.IsRunningOn
)

// networkIDResult carries the outcome of a single provider's network ID lookup.
type networkIDResult struct {
	provider  string
	networkID string
	err       error
}

// providerCheckResult carries the outcome of a single provider's IsRunningOn check.
type providerCheckResult struct {
	provider string
	ok       bool
}

// GetConfiguredNetworkID returns the network ID from configuration, if set.
// Callers can use this to short-circuit cloud provider detection entirely
// when a network ID has been explicitly configured.
func GetConfiguredNetworkID() (string, bool) {
	networkID := pkgconfigsetup.Datadog().GetString("network.id")
	return networkID, networkID != ""
}

// GetNetworkID retrieves the network_id which can be used to improve network
// connection resolution. The result is cached, so repeated calls (e.g. across
// retries by a caller) are cheap. Resolution order:
//   - configuration
//   - if the host's cloud provider can be positively identified (EC2, GCE,
//     Azure, or Oracle), only that provider's metadata endpoint is queried,
//     to avoid wasting calls on endpoints known not to apply to this host
//   - otherwise, GCE and EC2 are queried concurrently, and the first one to
//     succeed wins
func GetNetworkID(ctx context.Context) (string, error) {
	return cache.Get[string](networkIDCacheKey, func() (string, error) {
		if networkID, ok := GetConfiguredNetworkID(); ok {
			log.Debugf("GetNetworkID: using configured network ID: %s", networkID)
			return networkID, nil
		}
		// TODO: If detectCloudProvider fails, can we expect getNetworkIDForProvider
		// to succeed? If not, we could return early and ensure non AWS, GCP, OCI hosts
		// can start faster.
		if provider, err := detectCloudProvider(ctx); err == nil {
			return getNetworkIDForProvider(ctx, provider)
		}
		return raceNetworkIDProviders(ctx)
	})
}

// detectCloudProvider checks, in parallel, whether the host is running on
// EC2, GCE, or Oracle, and returns the name of the one that matched.
// EC2 and GCE are the only two providers this package knows how to resolve a
// network ID for; Oracle is included so hosts running on them can
// be identified and short-circuited instead of wastefully probing EC2 and GCE
// metadata endpoints. Azure, IBM, Alibaba, and Tencent are deliberately left out:
// unlike EC2/GCE/Azure/Oracle, their negative-case timeouts are large
// (notably IBM's), which would slow down the common case of a host that
// isn't on any of these clouds.
//
// A positive result is cached indefinitely, since a host's cloud provider
// doesn't change. An inconclusive result is not cached, so retrying callers
// try again in case IsRunningOn failed for a transient reason (e.g. IMDS not
// up yet at boot).
func detectCloudProvider(ctx context.Context) (string, error) {
	return cache.Get[string](networkIDProviderCacheKey, func() (string, error) {
		// if possible, cancel any still-in-flight check as soon as we have an answer
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		providers := []struct {
			name     string
			callback func(context.Context) bool
		}{
			{ec2.CloudProviderName, isRunningOnEC2},
			{gce.CloudProviderName, isRunningOnGCE},
			// {azure.CloudProviderName, isRunningOnAzure}, // Azure endpoint times out adding additional latency
			{oracle.CloudProviderName, isRunningOnOracle},
		}

		results := make(chan providerCheckResult, len(providers))
		for _, p := range providers {
			p := p
			go func() { results <- providerCheckResult{p.name, p.callback(ctx)} }()
		}
		for i := 0; i < len(providers); i++ {
			if res := <-results; res.ok {
				return res.provider, nil
			}
		}
		return "", errors.New("could not detect cloud provider")
	})
}

// raceNetworkIDProviders queries GCE and EC2 concurrently and returns the
// network ID from whichever one succeeds first.
func raceNetworkIDProviders(ctx context.Context) (string, error) {
	cfg := pkgconfigsetup.Datadog()

	// cancel any still-in-flight lookup as soon as we have a winner, so the
	// losing provider doesn't keep waiting out its full timeout in the background
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var providers []struct {
		name string
		fn   func(context.Context) (string, error)
	}
	if configutils.IsCloudProviderEnabled(gce.CloudProviderName, cfg) {
		providers = append(providers, struct {
			name string
			fn   func(context.Context) (string, error)
		}{gce.CloudProviderName, getGCENetworkID})
	}
	if configutils.IsCloudProviderEnabled(ec2.CloudProviderName, cfg) {
		providers = append(providers, struct {
			name string
			fn   func(context.Context) (string, error)
		}{ec2.CloudProviderName, getEC2NetworkID})
	}

	if len(providers) == 0 {
		return "", errors.New("cloud provider metadata is disabled by configuration")
	}

	results := make(chan networkIDResult, len(providers))
	for _, p := range providers {
		p := p
		go func() {
			log.Debugf("GetNetworkID trying %s", p.name)
			networkID, err := p.fn(ctx)
			results <- networkIDResult{provider: p.name, networkID: networkID, err: err}
		}()
	}

	// collect results in completion order (not provider-priority order), so
	// whichever provider answers first wins, regardless of list position
	var errs []error
	for i := 0; i < len(providers); i++ {
		res := <-results
		if res.err == nil {
			log.Debugf("GetNetworkID: using network ID from %s metadata: %s", res.provider, res.networkID)
			return res.networkID, nil
		}
		errs = append(errs, res.err)
	}

	return "", fmt.Errorf("could not detect network ID: %w", errors.Join(errs...))
}

// getNetworkIDForProvider queries only the given cloud provider's metadata
// endpoint instead of racing GCE and EC2 together. Used once the host's cloud
// provider has been positively identified, to avoid wasting a call on an
// endpoint known not to apply to this host.
func getNetworkIDForProvider(ctx context.Context, provider string) (string, error) {
	var fn func(context.Context) (string, error)
	switch provider {
	case gce.CloudProviderName:
		fn = getGCENetworkID
	case ec2.CloudProviderName:
		fn = getEC2NetworkID
	default:
		return "", fmt.Errorf("host is running on %s, which does not support network ID resolution", provider)
	}

	if !configutils.IsCloudProviderEnabled(provider, pkgconfigsetup.Datadog()) {
		return "", fmt.Errorf("cloud provider %s metadata is disabled by configuration", provider)
	}

	log.Debugf("GetNetworkID trying %s", provider)
	networkID, err := fn(ctx)
	if err != nil {
		return "", fmt.Errorf("could not detect network ID: %w", err)
	}
	log.Debugf("GetNetworkID: using network ID from %s metadata: %s", provider, networkID)
	return networkID, nil
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker

package docker

import (
	"fmt"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetDockerHostIPs returns the IP address of the host. This is meant to be called
// only when the agent is running in a dockerized environment.
func GetDockerHostIPs() []string {
	cacheKey := cache.BuildAgentKey("hostIPs")
	if cachedIPs, found := cache.Cache.Get(cacheKey); found {
		return cachedIPs.([]string)
	}

	ips := getDockerHostIPsUncached()
	if len(ips) == 0 {
		log.Warnf("could not get host IP")
	}
	cache.Cache.Set(cacheKey, ips, time.Hour*2)
	return ips
}

type hostIPProvider struct {
	name     string
	provider func() ([]string, error)
}

func getDockerHostIPsUncached() []string {
	providers := []hostIPProvider{
		{"config", getHostIPsFromConfig},
		{"ec2 metadata endpoint", ec2.GetLocalIPv4},
		//FIXME: Do we need a specific implementation for windows container?
		{"/proc/net/route", containers.DefaultHostIPs},
	}

	return tryProviders(providers)
}

func tryProviders(providers []hostIPProvider) []string {
	for _, attempt := range providers {
		log.Debugf("attempting to get host ip from source: %s", attempt.name)
		ips, err := attempt.provider()
		if err != nil {
			log.Infof("could not deduce host IP from source %s: %s", attempt.name, err)
		} else {
			return ips
		}
	}
	return nil
}

func getHostIPsFromConfig() ([]string, error) {
	hostIPs := config.Datadog.GetStringSlice("process_agent_config.host_ips")

	if len(hostIPs) == 0 {
		return nil, fmt.Errorf("no hostIPs were configured")
	}

	for _, ipStr := range hostIPs {
		if net.ParseIP(ipStr) == nil {
			return nil, fmt.Errorf("could not parse IP: %s", ipStr)
		}
	}

	return hostIPs, nil
}

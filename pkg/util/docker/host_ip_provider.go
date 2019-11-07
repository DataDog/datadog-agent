// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"fmt"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetDockerHostIP returns the IP address of the host. This is meant to be called
// only when the agent is running in a dockerized environment.
func GetDockerHostIP() []string {
	cacheKey := cache.BuildAgentKey("hostIPs")
	if cachedIPs, found := cache.Cache.Get(cacheKey); found {
		return cachedIPs.([]string)
	}

	ips := getDockerHostIPUncached()
	if len(ips) == 0 {
		log.Warnf("could not get host IP")
	}
	cache.Cache.Set(cacheKey, ips, time.Hour*2)
	return ips
}

func getDockerHostIPUncached() []string {
	type hostIPProvider struct {
		name     string
		provider func() ([]string, error)
	}

	var isHostMode bool
	if mode, err := GetAgentContainerNetworkMode(); err != nil && mode == "host" {
		isHostMode = true
	}

	var providers []hostIPProvider
	providers = append(providers, hostIPProvider{"config", getHostIPFFromConfig})
	providers = append(providers, hostIPProvider{"ec2 metadata endpoint", ec2.GetLocalIPv4})
	if isHostMode {
		providers = append(providers, hostIPProvider{"/proc/net/route", DefaultHostIPs})
	}

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

func getHostIPFFromConfig() ([]string, error) {
	hostIPs := config.Datadog.GetStringSlice("process_agent_config.host_ips")

	for _, ipStr := range hostIPs {
		if net.ParseIP(ipStr) == nil {
			return nil, fmt.Errorf("could not parse IP: %s", ipStr)
		}
	}

	return hostIPs, nil
}

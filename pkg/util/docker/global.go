// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/docker/docker/api/types/container"

	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

var (
	globalDockerUtil     *DockerUtil
	invalidationInterval = 5 * time.Minute
)

// GetDockerUtil returns a ready to use DockerUtil. It is backed by a shared singleton.
func GetDockerUtil() (*DockerUtil, error) {
	if globalDockerUtil == nil {
		globalDockerUtil = &DockerUtil{}
		globalDockerUtil.initRetry.SetupRetrier(&retry.Config{
			Name:          "dockerutil",
			AttemptMethod: globalDockerUtil.init,
			Strategy:      retry.RetryCount,
			RetryCount:    10,
			RetryDelay:    30 * time.Second,
		})
	}
	if err := globalDockerUtil.initRetry.TriggerRetry(); err != nil {
		log.Debugf("Docker init error: %s", err)
		return nil, err
	}
	return globalDockerUtil, nil
}

// EnableTestingMode creates a "mocked" DockerUtil you can use for unit
// tests that will hit on the docker inspect cache. Please note that all
// calls to the docker server will result in nil pointer exceptions.
func EnableTestingMode() {
	globalDockerUtil = &DockerUtil{}
	globalDockerUtil.initRetry.SetupRetrier(&retry.Config{
		Name:     "dockerutil",
		Strategy: retry.JustTesting,
	})
}

// HostnameProvider docker implementation for the hostname provider
func HostnameProvider() (string, error) {
	du, err := GetDockerUtil()
	if err != nil {
		return "", err
	}
	return du.GetHostname()
}

// GetAgentContainerNetworkMode provides the network mode of the Agent container
func GetAgentContainerNetworkMode() (string, error) {
	du, err := GetDockerUtil()
	if err != nil {
		return "", err
	}
	agentContainer, err := du.InspectSelf()
	if err != nil {
		return "", err
	}
	return parseContainerNetworkMode(agentContainer.HostConfig)
}

// parseContainerNetworkMode returns the network mode of a container
func parseContainerNetworkMode(hostConfig *container.HostConfig) (string, error) {
	if hostConfig == nil {
		return "", errors.New("the HostConfig field is nil")
	}
	mode := string(hostConfig.NetworkMode)
	switch mode {
	case containers.HostNetworkMode:
		return containers.HostNetworkMode, nil
	case containers.BridgeNetworkMode:
		return containers.BridgeNetworkMode, nil
	case containers.NoneNetworkMode:
		return containers.NoneNetworkMode, nil
	}
	if strings.HasPrefix(mode, "container:") {
		return containers.AwsvpcNetworkMode, nil
	}
	return "", fmt.Errorf("unknown network mode: %s", mode)
}

// Config is an exported configuration object that is used when
// initializing the DockerUtil.
type Config struct {
	// CacheDuration is the amount of time we will cache the active docker
	// containers and cgroups. The actual raw metrics (e.g. MemRSS) will _not_
	// be cached but will be re-calculated on all calls to AllContainers.
	CacheDuration time.Duration
	// CollectNetwork enables network stats collection. This requires at least
	// one call to container.Inspect for new containers and reads from the
	// procfs for stats.
	CollectNetwork bool
	// Whitelist is a slice of filter strings in the form of key:regex where key
	// is either 'image' or 'name' and regex is a valid regular expression.
	Whitelist []string
	// Blacklist is the same as whitelist but for exclusion.
	Blacklist []string

	// internal use only
	filter *containers.Filter
}

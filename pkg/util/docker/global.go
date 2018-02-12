// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"os"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

var (
	globalDockerUtil     *DockerUtil
	invalidationInterval = 5 * time.Minute
	lastErr              string

	// NullContainer is an empty container object that has
	// default values for all fields including sub-fields.
	// If new sub-structs are added to Container this must
	// be updated.
	NullContainer = &Container{
		CPU:     &CgroupTimesStat{},
		Memory:  &CgroupMemStat{},
		IO:      &CgroupIOStat{},
		Network: ContainerNetStats{},
	}
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
	err := globalDockerUtil.initRetry.TriggerRetry()
	if err != nil {
		log.Debugf("init error: %s", err)
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
func HostnameProvider(hostName string) (string, error) {
	du, err := GetDockerUtil()
	if err != nil {
		return "", err
	}
	return du.GetHostname()
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
	filter *containerFilter
}

// Expose module-level functions that will interact with a the globalDockerUtil singleton.
// These are to be deprecated in favor or directly calling the DockerUtil methods.

type ContainerListConfig struct {
	IncludeExited bool
	FlagExcluded  bool
}

func (cfg *ContainerListConfig) GetCacheKey() string {
	cacheKey := "dockerutil.containers"
	if cfg.IncludeExited {
		cacheKey += ".with_exited"
	} else {
		cacheKey += ".without_exited"
	}

	if cfg.FlagExcluded {
		cacheKey += ".with_excluded"
	} else {
		cacheKey += ".without_excluded"
	}

	return cacheKey
}

// IsContainerized returns True if we're running in the docker-dd-agent container.
func IsContainerized() bool {
	return os.Getenv("DOCKER_DD_AGENT") == "yes"
}

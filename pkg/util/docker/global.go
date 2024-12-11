// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

var (
	globalDockerUtil      *DockerUtil
	globalDockerUtilMutex sync.Mutex
)

// GetDockerUtilWithRetrier returns a ready to use DockerUtil or a retrier
func GetDockerUtilWithRetrier() (*DockerUtil, *retry.Retrier) {
	globalDockerUtilMutex.Lock()
	defer globalDockerUtilMutex.Unlock()
	if globalDockerUtil == nil {
		globalDockerUtil = &DockerUtil{}
		globalDockerUtil.initRetry.SetupRetrier(&retry.Config{ //nolint:errcheck
			Name:              "dockerutil",
			AttemptMethod:     globalDockerUtil.init,
			Strategy:          retry.Backoff,
			InitialRetryDelay: 1 * time.Second,
			MaxRetryDelay:     5 * time.Minute,
		})
	}
	if err := globalDockerUtil.initRetry.TriggerRetry(); err != nil {
		log.Debugf("Docker init error: %s", err)
		return nil, &globalDockerUtil.initRetry
	}
	return globalDockerUtil, nil
}

// GetDockerUtil returns a ready to use DockerUtil. It is backed by a shared singleton.
func GetDockerUtil() (*DockerUtil, error) {
	util, retirer := GetDockerUtilWithRetrier()
	if retirer != nil {
		return nil, retirer.LastError()
	}
	return util, nil
}

// EnableTestingMode creates a "mocked" DockerUtil you can use for unit
// tests that will hit on the docker inspect cache. Please note that all
// calls to the docker server will result in nil pointer exceptions.
func EnableTestingMode() {
	globalDockerUtilMutex.Lock()
	defer globalDockerUtilMutex.Unlock()
	globalDockerUtil = &DockerUtil{}
	globalDockerUtil.initRetry.SetupRetrier(&retry.Config{ //nolint:errcheck
		Name:     "dockerutil",
		Strategy: retry.JustTesting,
	})
}

// GetHostname returns the hostname for docker
func GetHostname(ctx context.Context) (string, error) {
	du, err := GetDockerUtil()
	if err != nil {
		return "", err
	}
	return du.GetHostname(ctx)
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

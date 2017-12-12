// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package docker

import (
	"errors"
	"os"
	"strings"
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

const (
	ContainerCreatedState    string = "created"
	ContainerRunningState    string = "running"
	ContainerRestartingState string = "restarting"
	ContainerPausedState     string = "paused"
	ContainerExitedState     string = "exited"
	ContainerDeadState       string = "dead"
)

// GetDockerUtil returns a ready to use DockerUtil. It is backed by a shared singleton.
func GetDockerUtil() (*DockerUtil, error) {
	if globalDockerUtil == nil {
		globalDockerUtil = &DockerUtil{}
		globalDockerUtil.SetupRetrier(&retry.Config{
			Name:          "dockerutil",
			AttemptMethod: globalDockerUtil.init,
			Strategy:      retry.RetryCount,
			RetryCount:    10,
			RetryDelay:    30 * time.Second,
		})
	}
	err := globalDockerUtil.TriggerRetry()
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
	globalDockerUtil.SetupRetrier(&retry.Config{
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

// Container represents a single Docker container on a machine
// and includes Cgroup-level statistics about the container.
type Container struct {
	Type     string
	ID       string
	EntityID string
	Name     string
	Image    string
	ImageID  string
	Created  int64
	State    string
	Health   string
	Pids     []int32
	Excluded bool

	CPULimit       float64
	MemLimit       uint64
	CPUNrThrottled uint64
	CPU            *CgroupTimesStat
	Memory         *CgroupMemStat
	IO             *CgroupIOStat
	Network        ContainerNetStats
	StartedAt      int64

	// For internal use only
	cgroup *ContainerCgroup
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

//
// Expose module-level functions that will interact with a the globalDockerUtil singleton.
// These are to be deprecated in favor or directly calling the DockerUtil methods.
//

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

// GetInspectCacheKey returns the key to a given container ID inspect in the agent cache
func GetInspectCacheKey(ID string) string {
	return "dockerutil.containers." + ID
}

// SplitImageName splits a valid image name (from ResolveImageName) and returns:
//    - the "long image name" with registry and prefix, without tag
//    - the "short image name", without registry, prefix nor tag
//    - the image tag if present
//    - an error if parsing failed
func SplitImageName(image string) (string, string, string, error) {
	// See TestSplitImageName for supported formats (number 6 will surprise you!)
	if image == "" {
		return "", "", "", errors.New("empty image name")
	}
	long := image
	if pos := strings.LastIndex(long, "@sha"); pos > 0 {
		// Remove @sha suffix when orchestrator is sha-pinning
		long = long[0:pos]
	}

	var short, tag string
	last_colon := strings.LastIndex(long, ":")
	last_slash := strings.LastIndex(long, "/")

	if last_colon > -1 && last_colon > last_slash {
		// We have a tag
		tag = long[last_colon+1:]
		long = long[:last_colon]
	}
	if last_slash > -1 {
		// we have a prefix / registry
		short = long[last_slash+1:]
	} else {
		short = long
	}
	return long, short, tag, nil
}

// IsContainerized returns True if we're running in the docker-dd-agent container.
func IsContainerized() bool {
	return os.Getenv("DOCKER_DD_AGENT") == "yes"
}

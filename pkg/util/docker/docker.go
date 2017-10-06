// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package docker

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

var (
	// ErrDockerNotAvailable is returned if Docker is not running on the current machine.
	// We'll use this when configuring the DockerUtil so we don't error on non-docker machines.
	ErrDockerNotAvailable = errors.New("docker not available")

	globalDockerUtil     *dockerUtil
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
		Network: &NetworkStat{},
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

// NetworkStat stores network statistics about a Docker container.
type NetworkStat struct {
	BytesSent   uint64
	BytesRcvd   uint64
	PacketsSent uint64
	PacketsRcvd uint64
}

type containerFilter struct {
	Enabled        bool
	ImageWhitelist []*regexp.Regexp
	NameWhitelist  []*regexp.Regexp
	ImageBlacklist []*regexp.Regexp
	NameBlacklist  []*regexp.Regexp
}

// HostnameProvider docker implementation for the hostname provider
func HostnameProvider(hostName string) (string, error) {
	return GetHostname()
}

// DefaultGateway returns the default Docker gateway.
func DefaultGateway() (net.IP, error) {
	procRoot := config.Datadog.GetString("proc_root")
	netRouteFile := filepath.Join(procRoot, "net", "route")
	f, err := os.Open(netRouteFile)
	if os.IsNotExist(err) || os.IsPermission(err) {
		log.Errorf("unable to open %s: %s", netRouteFile, err)
		return nil, nil
	} else if err != nil {
		// Unknown error types will bubble up for handling.
		return nil, err
	}
	defer f.Close()

	ip := make(net.IP, 4)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && fields[1] == "00000000" {
			ipInt, err := strconv.ParseInt(fields[2], 16, 32)
			if err != nil {
				return nil, fmt.Errorf("unable to parse ip %s, from %s: %s", fields[2], netRouteFile, err)
			}
			binary.LittleEndian.PutUint32(ip, uint32(ipInt))
			break
		}
	}
	return ip, nil
}

// IsExcluded returns a bool indicating if the container should be excluded
// based on the filters in the containerFilter instance.
func (cf containerFilter) IsExcluded(container *Container) bool {
	return cf.computeIsExcluded(container.Name, container.Image)
}

func (cf containerFilter) computeIsExcluded(containerName, containerImage string) bool {
	if !cf.Enabled {
		return false
	}

	var excluded bool
	for _, r := range cf.ImageBlacklist {
		if r.MatchString(containerImage) {
			excluded = true
			break
		}
	}
	for _, r := range cf.NameBlacklist {
		if r.MatchString(containerName) {
			excluded = true
			break
		}
	}

	// Any excluded container could be whitelisted.
	if excluded {
		for _, r := range cf.ImageWhitelist {
			if r.MatchString(containerImage) {
				return false
			}
		}
		for _, r := range cf.NameWhitelist {
			if r.MatchString(containerName) {
				return false
			}
		}
	}
	return excluded
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
	Network        *NetworkStat
	StartedAt      int64

	// For internal use only
	cgroup *ContainerCgroup
}

// Inspect allows getting the full docker inspect of a Container
func (c *Container) Inspect(withSize bool) (types.ContainerJSON, error) {
	cj, err := Inspect(c.ID, withSize)
	return cj, err
}

// Inspect returns a docker inspect object for a given container ID.
// It tries to locate the container in the inspect cache before making the docker inspect call
func Inspect(id string, withSize bool) (types.ContainerJSON, error) {
	cacheKey := GetInspectCacheKey(id)
	var container types.ContainerJSON
	var err error
	var ok bool

	if cached, hit := cache.Cache.Get(cacheKey); hit {
		container, ok = cached.(types.ContainerJSON)
		if !ok {
			log.Errorf("invalid cache format, forcing a cache miss")
		}
	} else {
		if globalDockerUtil == nil {
			return types.ContainerJSON{}, fmt.Errorf("DockerUtil not initialized")
		}
		container, _, err = globalDockerUtil.cli.ContainerInspectWithRaw(context.Background(), id, withSize)
		// cache the inspect for 10 seconds to reduce pressure on the daemon
		cache.Cache.Set(cacheKey, container, 10*time.Second)
	}

	return container, err
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
// Expose module-level functions that will interact with a Singleton dockerUtil.

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

// AllContainers returns a slice of all containers.
func AllContainers(cfg *ContainerListConfig) ([]*Container, error) {
	if globalDockerUtil != nil {
		r, err := globalDockerUtil.containers(cfg)
		if err != nil {
			return nil, log.Errorf("unable to list Docker containers: %s", err)
		}
		return r, nil
	}
	return nil, ErrDockerNotAvailable
}

// AllImages returns a slice of all images.
func AllImages(includeIntermediate bool) ([]types.ImageSummary, error) {
	if globalDockerUtil != nil {
		return globalDockerUtil.dockerImages(includeIntermediate)
	}
	return nil, nil
}

// ResolveImageName resolves a docker image name, probably containing
// sha256 checksum as tag in a name:tag format string.
// This requires InitDockerUtil to be called before.
func ResolveImageName(image string) (string, error) {
	if globalDockerUtil != nil {
		return globalDockerUtil.extractImageName(image), nil
	}
	return "", errors.New("dockerutil not initialised")
}

// SplitImageName splits a valid image name (from ResolveImageName)
// into the name and tag parts.
func SplitImageName(image string) (string, string, error) {
	if image == "" {
		return "", "", errors.New("empty image name")
	}
	parts := strings.SplitN(image, ":", 2)
	if len(parts) < 2 {
		return image, "", errors.New("could not find tag")
	}
	return parts[0], parts[1], nil
}

// GetHostname returns the Docker hostname.
func GetHostname() (string, error) {
	if globalDockerUtil == nil {
		return "", ErrDockerNotAvailable
	}
	return globalDockerUtil.getHostname()
}

// IsContainerized returns True if we're running in the docker-dd-agent container.
func IsContainerized() bool {
	return os.Getenv("DOCKER_DD_AGENT") == "yes"
}

// IsAvailable returns true if Docker is available on this machine via a socket.
func IsAvailable() bool {
	if _, err := ConnectToDocker(); err != nil {
		if err != ErrDockerNotAvailable {
			log.Warnf("unable to connect to docker: %s", err)
		}
		return false
	}
	return true
}

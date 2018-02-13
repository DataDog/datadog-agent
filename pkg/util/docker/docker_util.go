// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/client"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	// pauseContainerGCR regex matches:
	// - k8s.gcr.io/pause-amd64:3.1
	// - asia.gcr.io/google_containers/pause-amd64:3.0
	// - gcr.io/google_containers/pause-amd64:3.0
	pauseContainerGCR        = `image:(.*)gcr\.io(/google_containers/|/)pause(.*)`
	pauseContainerOpenshift  = "image:openshift/origin-pod"
	pauseContainerKubernetes = "image:kubernetes/pause"
)

// FIXME: remove once DockerListener is moved to .Containers
func (d *DockerUtil) ContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
	return d.cli.ContainerList(ctx, options)
}

// DockerUtil wraps interactions with a local docker API.
type DockerUtil struct {
	// used to setup the DockerUtil
	initRetry retry.Retrier

	sync.Mutex
	cfg *Config
	cli *client.Client
	// tracks the last time we invalidate our internal caches
	lastInvalidate time.Time
	// networkMappings by container id
	networkMappings map[string][]dockerNetwork
	// image sha mapping cache
	imageNameBySha map[string]string
	// event subscribers and state
	eventState *eventStreamState
}

// init makes an empty DockerUtil bootstrap itself.
// This is not exposed as public API but is called by the retrier embed.
func (d *DockerUtil) init() error {
	// Major failure risk is here, do that first
	cli, err := ConnectToDocker()
	if err != nil {
		return err
	}

	cfg := &Config{
		// TODO: bind them to config entries if relevant
		CollectNetwork: true,
		CacheDuration:  10 * time.Second,
	}

	whitelist := config.Datadog.GetStringSlice("ac_include")
	blacklist := config.Datadog.GetStringSlice("ac_exclude")

	if config.Datadog.GetBool("exclude_pause_container") {
		blacklist = append(blacklist, pauseContainerGCR, pauseContainerOpenshift, pauseContainerKubernetes)
	}

	// Pre-parse the filter and use that internally.
	cfg.filter, err = newContainerFilter(whitelist, blacklist)
	if err != nil {
		return err
	}

	d.cfg = cfg
	d.cli = cli
	d.networkMappings = make(map[string][]dockerNetwork)
	d.imageNameBySha = make(map[string]string)
	d.lastInvalidate = time.Now()
	d.eventState = newEventStreamState()

	return nil
}

// ConnectToDocker connects to a local docker socket.
// Returns ErrDockerNotAvailable if the socket or mounts file is missing
// otherwise it returns either a valid client or an error.
//
// TODO: REMOVE USES AND MOVE TO PRIVATE
//
func ConnectToDocker() (*client.Client, error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		return nil, err
	}
	clientVersion := cli.ClientVersion()
	cli.UpdateClientVersion("") // Hit unversionned endpoint first

	// TODO: remove this logic when "client.NegotiateAPIVersion" function is released by moby/docker
	v, err := cli.ServerVersion(context.Background())
	if err != nil || v.APIVersion == "" {
		return nil, fmt.Errorf("could not determine docker server API version: %s", err)
	}
	serverVersion := v.APIVersion

	if versions.LessThan(serverVersion, clientVersion) {
		log.Debugf("Docker server APIVersion ('%s') is lower than the client ('%s'): using version from the server",
			serverVersion, clientVersion)
		cli.UpdateClientVersion(serverVersion)
	} else {
		cli.UpdateClientVersion(clientVersion)
	}

	log.Debugf("Successfully connected to Docker server version %s", v.Version)

	return cli, nil
}

// Images returns a slice of all images.
func (d *DockerUtil) Images(includeIntermediate bool) ([]types.ImageSummary, error) {
	images, err := d.cli.ImageList(context.Background(), types.ImageListOptions{All: includeIntermediate})
	if err != nil {
		return nil, fmt.Errorf("unable to list docker images: %s", err)
	}
	return images, nil
}

// CountVolumes returns the number of attached and dangling volumes.
func (d *DockerUtil) CountVolumes() (int, int, error) {
	attachedFilter, _ := buildDockerFilter("dangling", "false")
	danglingFilter, _ := buildDockerFilter("dangling", "true")

	attachedVolumes, err := d.cli.VolumeList(context.Background(), attachedFilter)
	if err != nil {
		return 0, 0, fmt.Errorf("unable to list attached docker volumes: %s", err)
	}
	danglingVolumes, err := d.cli.VolumeList(context.Background(), danglingFilter)
	if err != nil {
		return 0, 0, fmt.Errorf("unable to list dangling docker volumes: %s", err)
	}

	return len(attachedVolumes.Volumes), len(danglingVolumes.Volumes), nil
}

// dockerContainers returns a list of Docker info for active containers using the
// Docker API. This requires the running user to be in the "docker" user group
// or have access to /tmp/docker.sock.
func (d *DockerUtil) dockerContainers(cfg *ContainerListConfig) ([]*Container, error) {
	containers, err := d.cli.ContainerList(context.Background(), types.ContainerListOptions{All: cfg.IncludeExited})
	if err != nil {
		return nil, fmt.Errorf("error listing containers: %s", err)
	}
	ret := make([]*Container, 0, len(containers))
	for _, c := range containers {
		if d.cfg.CollectNetwork {
			// FIXME: We might need to invalidate this cache if a containers networks are changed live.
			d.Lock()
			if _, ok := d.networkMappings[c.ID]; !ok {
				i, err := d.cli.ContainerInspect(context.Background(), c.ID)
				if err != nil && client.IsErrContainerNotFound(err) {
					d.Unlock()
					log.Debugf("Error inspecting container %s: %s", c.ID, err)
					continue
				}
				d.networkMappings[c.ID] = findDockerNetworks(c.ID, i.State.Pid, c)
			}
			d.Unlock()
		}

		image, err := d.ResolveImageName(c.Image)
		if err != nil {
			log.Warnf("Can't resolve image name %s: %s", c.Image, err)
		}

		entityID := fmt.Sprintf("docker://%s", c.ID)
		container := &Container{
			Type:     "Docker",
			ID:       entityID[9:],
			EntityID: entityID,
			Name:     c.Names[0],
			Image:    image,
			ImageID:  c.ImageID,
			Created:  c.Created,
			State:    c.State,
			Health:   parseContainerHealth(c.Status),
		}

		container.Excluded = d.cfg.filter.IsExcluded(container)
		if container.Excluded && !cfg.FlagExcluded {
			continue
		}
		ret = append(ret, container)
	}

	// Resolve docker networks after we've processed all containers so all
	// routing maps are available.
	if d.cfg.CollectNetwork {
		d.Lock()
		resolveDockerNetworks(d.networkMappings)
		d.Unlock()
	}

	if d.lastInvalidate.Add(invalidationInterval).After(time.Now()) {
		d.invalidateCaches(containers)
	}

	return ret, nil
}

// Containers gets a list of all containers on the current node using a mix of
// the Docker APIs and cgroups stats. We attempt to limit syscalls where possible.
func (d *DockerUtil) Containers(cfg *ContainerListConfig) ([]*Container, error) {
	cacheKey := cfg.GetCacheKey()

	// Get the containers either from our cache or with API queries.
	var containers []*Container
	cached, hit := cache.Cache.Get(cacheKey)
	if hit {
		var ok bool
		containers, ok = cached.([]*Container)
		if !ok {
			log.Errorf("Invalid container list cache format, forcing a cache miss")
			hit = false
		}
	}
	if !hit {
		var cgByContainer map[string]*ContainerCgroup
		var err error

		cgByContainer, err = ScrapeAllCgroups()
		if err != nil {
			return nil, fmt.Errorf("could not get cgroups: %s", err)
		}

		containers, err = d.dockerContainers(cfg)
		if err != nil {
			return nil, fmt.Errorf("could not get docker containers: %s", err)
		}

		for _, container := range containers {
			if container.Excluded {
				continue
			}
			cgroup, ok := cgByContainer[container.ID]
			if !ok {
				continue
			}
			container.cgroup = cgroup
			container.CPULimit, err = cgroup.CPULimit()
			if err != nil {
				log.Debugf("Cgroup cpu limit: %s", err)
			}
			container.MemLimit, err = cgroup.MemLimit()
			if err != nil {
				log.Debugf("Cgroup cpu limit: %s", err)
			}
		}
		cache.Cache.Set(cacheKey, containers, d.cfg.CacheDuration)
	}

	// Fill in the latest statistics from the cgroups
	// Creating a new list of containers with copies so we don't lose
	// the previous state for calculations (e.g. last cpu).
	var err error
	newContainers := make([]*Container, 0, len(containers))
	for _, lastContainer := range containers {
		if (cfg.IncludeExited && lastContainer.State == ContainerExitedState) || lastContainer.Excluded {
			newContainers = append(newContainers, lastContainer)
			continue
		}

		container := &Container{}
		*container = *lastContainer

		cgroup := container.cgroup
		if cgroup == nil {
			log.Debugf("Container id %s has an empty cgroup, skipping", container.ID[:12])
			continue
		}

		container.Memory, err = cgroup.Mem()
		if err != nil {
			log.Debugf("Cgroup memory: %s", err)
			continue
		}
		container.CPU, err = cgroup.CPU()
		if err != nil {
			log.Debugf("Cgroup cpu: %s", err)
			continue
		}
		container.CPUNrThrottled, err = cgroup.CPUNrThrottled()
		if err != nil {
			log.Debugf("Cgroup cpuNrThrottled: %s", err)
			continue
		}
		container.IO, err = cgroup.IO()
		if err != nil {
			log.Debugf("Cgroup i/o: %s", err)
			continue
		}

		if d.cfg.CollectNetwork {
			d.Lock()
			networks, ok := d.networkMappings[cgroup.ContainerID]
			d.Unlock()
			if ok && len(cgroup.Pids) > 0 {
				netStat, err := collectNetworkStats(cgroup.ContainerID, int(cgroup.Pids[0]), networks)
				if err != nil {
					log.Debugf("Could not collect network stats for container %s: %s", container.ID, err)
					continue
				}
				container.Network = netStat
			}
		} else {
			container.Network = NullContainer.Network
		}

		startedAt, err := cgroup.ContainerStartTime()
		if err != nil {
			log.Debugf("Failed to get container start time: %s", err)
			continue
		}
		container.StartedAt = startedAt
		container.Pids = cgroup.Pids

		newContainers = append(newContainers, container)
	}

	return newContainers, nil
}

func (d *DockerUtil) GetHostname() (string, error) {
	info, err := d.cli.Info(context.Background())
	if err != nil {
		return "", fmt.Errorf("unable to get Docker info: %s", err)
	}
	return info.Name, nil
}

// GetStorageStats returns the docker global storage stats if available
// or ErrStorageStatsNotAvailable
func (d *DockerUtil) GetStorageStats() ([]*StorageStats, error) {
	info, err := d.cli.Info(context.Background())
	if err != nil {
		return []*StorageStats{}, fmt.Errorf("unable to get Docker info: %s", err)
	}
	return parseStorageStatsFromInfo(info)
}

// ResolveImageName will resolve sha image name to their user-friendly name.
// For non-sha names we will just return the name as-is.
func (d *DockerUtil) ResolveImageName(image string) (string, error) {
	if !strings.Contains(image, "sha256:") {
		return image, nil
	}

	d.Lock()
	defer d.Unlock()
	if _, ok := d.imageNameBySha[image]; !ok {
		r, _, err := d.cli.ImageInspectWithRaw(context.Background(), image)
		if err != nil {
			// Only log errors that aren't "not found" because some images may
			// just not be available in docker inspect.
			if !client.IsErrNotFound(err) {
				return image, err
			}
			d.imageNameBySha[image] = image
		}

		// Try RepoTags first and fall back to RepoDigest otherwise.
		if len(r.RepoTags) > 0 {
			d.imageNameBySha[image] = r.RepoTags[0]
		} else if len(r.RepoDigests) > 0 {
			// Digests formatted like quay.io/foo/bar@sha256:hash
			sp := strings.SplitN(r.RepoDigests[0], "@", 2)
			d.imageNameBySha[image] = sp[0]
		} else {
			d.imageNameBySha[image] = image
		}
	}
	return d.imageNameBySha[image], nil
}

// Inspect returns a docker inspect object for a given container ID.
// It tries to locate the container in the inspect cache before making the docker inspect call
// TODO: try sized inspect if withSize=false and unsized key cache misses
func (d *DockerUtil) Inspect(id string, withSize bool) (types.ContainerJSON, error) {
	cacheKey := GetInspectCacheKey(id, withSize)
	var container types.ContainerJSON

	cached, hit := cache.Cache.Get(cacheKey)
	if hit {
		container, ok := cached.(types.ContainerJSON)
		if !ok {
			log.Errorf("Invalid inspect cache format, forcing a cache miss")
		} else {
			return container, nil
		}
	}
	container, _, err := d.cli.ContainerInspectWithRaw(context.Background(), id, withSize)
	if err != nil {
		return container, err
	}
	// ContainerJSONBase is a pointer embed, so it might be nil and cause segfaults
	if container.ContainerJSONBase == nil {
		return container, errors.New("invalid inspect data")
	}
	// cache the inspect for 10 seconds to reduce pressure on the daemon
	cache.Cache.Set(cacheKey, container, 10*time.Second)

	return container, nil
}

// Inspect detect the container ID we are running in and returns the inspect contents.
func (d *DockerUtil) InspectSelf() (types.ContainerJSON, error) {
	cID, _, err := readCgroupPaths("/proc/self/cgroup")
	if err != nil {
		return types.ContainerJSON{}, err
	}

	return d.Inspect(cID, false)
}

func (d *DockerUtil) invalidateCaches(containers []types.Container) {
	liveContainers := make(map[string]struct{})
	liveImages := make(map[string]struct{})
	for _, c := range containers {
		liveContainers[c.ID] = struct{}{}
		liveImages[c.Image] = struct{}{}
	}
	d.Lock()
	for cid := range d.networkMappings {
		if _, ok := liveContainers[cid]; !ok {
			delete(d.networkMappings, cid)
		}
	}
	for image := range d.imageNameBySha {
		if _, ok := liveImages[image]; !ok {
			delete(d.imageNameBySha, image)
		}
	}
	d.Unlock()
}

var healthRe = regexp.MustCompile(`\(health: (\w+)\)`)

// Parse the health out of a container status. The format is either:
//  - 'Up 5 seconds (health: starting)'
//  - 'Up about an hour'
//
func parseContainerHealth(status string) string {
	// Avoid allocations in most cases by just checking for '('
	if strings.IndexByte(status, '(') == -1 {
		return ""
	}
	all := healthRe.FindAllStringSubmatch(status, -1)
	if len(all) < 1 || len(all[0]) < 2 {
		return ""
	}
	return all[0][1]
}

// AllContainerLabels retrieves all running containers (`docker ps`) and returns
// a map mapping containerID to container labels as a map[string]string
func (d *DockerUtil) AllContainerLabels() (map[string]map[string]string, error) {
	containers, err := d.cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing containers: %s", err)
	}

	labelMap := make(map[string]map[string]string)

	for _, container := range containers {
		if len(container.ID) == 0 {
			continue
		}
		labelMap[container.ID] = container.Labels
	}

	return labelMap, nil
}

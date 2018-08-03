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

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/client"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// DockerUtil wraps interactions with a local docker API.
type DockerUtil struct {
	// used to setup the DockerUtil
	initRetry retry.Retrier

	sync.Mutex
	cfg          *Config
	cli          *client.Client
	queryTimeout time.Duration
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
	d.queryTimeout = config.Datadog.GetDuration("docker_query_timeout") * time.Second

	// Major failure risk is here, do that first
	ctx, cancel := context.WithTimeout(context.Background(), d.queryTimeout)
	defer cancel()
	cli, err := connectToDocker(ctx)
	if err != nil {
		return err
	}

	cfg := &Config{
		// TODO: bind them to config entries if relevant
		CollectNetwork: true,
		CacheDuration:  10 * time.Second,
	}

	cfg.filter, err = containers.NewFilterFromConfig()
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

// connectToDocker connects to docker and negociates the API version
func connectToDocker(ctx context.Context) (*client.Client, error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		return nil, err
	}
	clientVersion := cli.ClientVersion()
	cli.UpdateClientVersion("") // Hit unversionned endpoint first

	// TODO: remove this logic when "client.NegotiateAPIVersion" function is released by moby/docker
	v, err := cli.ServerVersion(ctx)
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
	ctx, cancel := context.WithTimeout(context.Background(), d.queryTimeout)
	defer cancel()
	images, err := d.cli.ImageList(ctx, types.ImageListOptions{All: includeIntermediate})

	if err != nil {
		return nil, fmt.Errorf("unable to list docker images: %s", err)
	}
	return images, nil
}

// CountVolumes returns the number of attached and dangling volumes.
func (d *DockerUtil) CountVolumes() (int, int, error) {
	attachedFilter, _ := buildDockerFilter("dangling", "false")
	danglingFilter, _ := buildDockerFilter("dangling", "true")
	ctx, cancel := context.WithTimeout(context.Background(), d.queryTimeout)
	defer cancel()

	attachedVolumes, err := d.cli.VolumeList(ctx, attachedFilter)
	if err != nil {
		return 0, 0, fmt.Errorf("unable to list attached docker volumes: %s", err)
	}
	danglingVolumes, err := d.cli.VolumeList(ctx, danglingFilter)
	if err != nil {
		return 0, 0, fmt.Errorf("unable to list dangling docker volumes: %s", err)
	}

	return len(attachedVolumes.Volumes), len(danglingVolumes.Volumes), nil
}

// dockerContainers returns a list of Docker info for active containers using the
// Docker API. This requires the running user to be in the "docker" user group
// or have access to /tmp/docker.sock.
func (d *DockerUtil) dockerContainers(cfg *ContainerListConfig) ([]*containers.Container, error) {
	if cfg == nil {
		return nil, errors.New("configuration is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), d.queryTimeout)
	defer cancel()
	cList, err := d.cli.ContainerList(ctx, types.ContainerListOptions{All: cfg.IncludeExited})
	if err != nil {
		return nil, fmt.Errorf("error listing containers: %s", err)
	}
	ret := make([]*containers.Container, 0, len(cList))
	for _, c := range cList {
		if d.cfg.CollectNetwork {
			// FIXME: We might need to invalidate this cache if a containers networks are changed live.
			d.Lock()
			if _, ok := d.networkMappings[c.ID]; !ok {
				i, err := d.Inspect(c.ID, false)
				if err != nil {
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

		excluded := d.cfg.filter.IsExcluded(c.Names[0], image)
		if excluded && !cfg.FlagExcluded {
			continue
		}

		entityID := fmt.Sprintf("docker://%s", c.ID)
		container := &containers.Container{
			Type:     "Docker",
			ID:       c.ID,
			EntityID: entityID,
			Name:     c.Names[0],
			Image:    image,
			ImageID:  c.ImageID,
			Created:  c.Created,
			State:    c.State,
			Excluded: excluded,
			Health:   parseContainerHealth(c.Status),
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
		d.invalidateCaches(cList)
	}

	return ret, nil
}

// Containers gets a list of all containers on the current node using a mix of
// the Docker APIs and cgroups stats. We attempt to limit syscalls where possible.
func (d *DockerUtil) Containers(cfg *ContainerListConfig) ([]*containers.Container, error) {
	cacheKey := cfg.GetCacheKey()

	// Get the containers either from our cache or with API queries.
	var cList []*containers.Container
	cached, hit := cache.Cache.Get(cacheKey)
	if hit {
		var ok bool
		cList, ok = cached.([]*containers.Container)
		if !ok {
			log.Errorf("Invalid container list cache format, forcing a cache miss")
			hit = false
		}
	}
	if !hit {
		cgByContainer, err := metrics.ScrapeAllCgroups()
		if err != nil {
			return nil, fmt.Errorf("could not get cgroups: %s", err)
		}

		cList, err = d.dockerContainers(cfg)
		if err != nil {
			return nil, fmt.Errorf("could not get docker containers: %s", err)
		}

		for _, container := range cList {
			if container.Excluded {
				continue
			}
			cgroup, ok := cgByContainer[container.ID]
			if !ok {
				continue
			}
			container.SetCgroups(cgroup)
			err = container.FillCgroupLimits()
			if err != nil {
				log.Debugf("Cannot get limits for container %s: %s, skipping", container.ID[:12], err)
				continue
			}
		}
		cache.Cache.Set(cacheKey, cList, d.cfg.CacheDuration)
	}

	// Fill in the latest statistics from the cgroups
	// Creating a new list of containers with copies so we don't lose
	// the previous state for calculations (e.g. last cpu).
	var err error
	newContainers := make([]*containers.Container, 0, len(cList))
	for _, lastContainer := range cList {
		if (cfg.IncludeExited && lastContainer.State == containers.ContainerExitedState) || lastContainer.Excluded {
			newContainers = append(newContainers, lastContainer)
			continue
		}

		container := &containers.Container{}
		*container = *lastContainer

		err = container.FillCgroupMetrics()
		if err != nil {
			log.Debugf("Cannot get metrics for container %s: %s", container.ID[:12], err)
			continue
		}

		if d.cfg.CollectNetwork {
			d.Lock()
			networks := d.networkMappings[container.ID]
			d.Unlock()

			nwByIface := make(map[string]string)
			for _, nw := range networks {
				nwByIface[nw.iface] = nw.dockerName
			}

			err = container.FillNetworkMetrics(nwByIface)
			if err != nil {
				log.Debugf("Cannot get network stats for container %s: %s", container.ID, err)
				continue
			}
		}

		newContainers = append(newContainers, container)
	}

	return newContainers, nil
}

// RawContainerList wraps around the docker client's ContainerList method.
// Value validation and error handling are the caller's responsibility.
func (d *DockerUtil) RawContainerList(options types.ContainerListOptions) ([]types.Container, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.queryTimeout)
	defer cancel()
	return d.cli.ContainerList(ctx, options)
}

func (d *DockerUtil) GetHostname() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.queryTimeout)
	defer cancel()
	info, err := d.cli.Info(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to get Docker info: %s", err)
	}
	return info.Name, nil
}

// GetStorageStats returns the docker global storage stats if available
// or ErrStorageStatsNotAvailable
func (d *DockerUtil) GetStorageStats() ([]*StorageStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.queryTimeout)
	defer cancel()
	info, err := d.cli.Info(ctx)
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
		ctx, cancel := context.WithTimeout(context.Background(), d.queryTimeout)
		defer cancel()
		r, _, err := d.cli.ImageInspectWithRaw(ctx, image)
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
	ctx, cancel := context.WithTimeout(context.Background(), d.queryTimeout)
	defer cancel()
	container, _, err := d.cli.ContainerInspectWithRaw(ctx, id, withSize)
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
	cID, _, err := metrics.ReadCgroupsForPath("/proc/self/cgroup")
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
	ctx, cancel := context.WithTimeout(context.Background(), d.queryTimeout)
	defer cancel()
	containers, err := d.cli.ContainerList(ctx, types.ContainerListOptions{})
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

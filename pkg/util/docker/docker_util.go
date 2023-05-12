// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"

	"github.com/DataDog/datadog-agent/pkg/config"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
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
	cli, err := ConnectToDocker(ctx)
	if err != nil {
		return err
	}

	cfg := &Config{
		// TODO: bind them to config entries if relevant
		CollectNetwork: true,
		CacheDuration:  10 * time.Second,
	}

	cfg.filter, err = containers.GetSharedMetricFilter()
	if err != nil {
		return err
	}

	d.cfg = cfg
	d.cli = cli
	d.imageNameBySha = make(map[string]string)
	d.lastInvalidate = time.Now()
	d.eventState = newEventStreamState()

	return nil
}

// ConnectToDocker connects to docker and negotiates the API version
func ConnectToDocker(ctx context.Context) (*client.Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	// Looks like docker is not actually doing a call to server when `NewClient` is called
	// Forcing it to verify server availability by calling Info()
	_, err = cli.Info(ctx)
	if err != nil {
		return nil, err
	}

	log.Debugf("Successfully connected to Docker server")

	return cli, nil
}

// Images returns a slice of all images.
func (d *DockerUtil) Images(ctx context.Context, includeIntermediate bool) ([]types.ImageSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, d.queryTimeout)
	defer cancel()
	images, err := d.cli.ImageList(ctx, types.ImageListOptions{All: includeIntermediate})
	if err != nil {
		return nil, fmt.Errorf("unable to list docker images: %s", err)
	}
	return images, nil
}

// CountVolumes returns the number of attached and dangling volumes.
func (d *DockerUtil) CountVolumes(ctx context.Context) (int, int, error) {
	attachedFilter, _ := buildDockerFilter("dangling", "false")
	danglingFilter, _ := buildDockerFilter("dangling", "true")
	ctx, cancel := context.WithTimeout(ctx, d.queryTimeout)
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

// RawClient returns the underlying docker client being used by this object.
func (d *DockerUtil) RawClient() *client.Client {
	return d.cli
}

// RawContainerList wraps around the docker client's ContainerList method.
// Value validation and error handling are the caller's responsibility.
func (d *DockerUtil) RawContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
	ctx, cancel := context.WithTimeout(ctx, d.queryTimeout)
	defer cancel()
	return d.cli.ContainerList(ctx, options)
}

// RawContainerListWithFilter is like RawContainerList but with a container filter.
func (d *DockerUtil) RawContainerListWithFilter(ctx context.Context, options types.ContainerListOptions, filter *containers.Filter) ([]types.Container, error) {
	containers, err := d.RawContainerList(ctx, options)
	if err != nil {
		return nil, err
	}

	if filter == nil {
		return containers, nil
	}

	isExcluded := func(container types.Container) bool {
		var annotations map[string]string
		if pod, err := workloadmeta.GetGlobalStore().GetKubernetesPodForContainer(container.ID); err == nil {
			annotations = pod.Annotations
		}
		for _, name := range container.Names {
			if filter.IsExcluded(annotations, name, container.Image, "") {
				log.Tracef("Container with name %q and image %q is filtered-out", name, container.Image)
				return true
			}
		}

		return false
	}

	filtered := []types.Container{}
	for _, container := range containers {
		if !isExcluded(container) {
			filtered = append(filtered, container)
		}
	}

	return filtered, nil
}

// GetHostname returns the hostname from the docker api
func (d *DockerUtil) GetHostname(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, d.queryTimeout)
	defer cancel()
	info, err := d.cli.Info(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to get Docker info: %s", err)
	}
	return info.Name, nil
}

// GetStorageStats returns the docker global storage stats if available
// or ErrStorageStatsNotAvailable
func (d *DockerUtil) GetStorageStats(ctx context.Context) ([]*StorageStats, error) {
	ctx, cancel := context.WithTimeout(ctx, d.queryTimeout)
	defer cancel()
	info, err := d.cli.Info(ctx)
	if err != nil {
		return []*StorageStats{}, fmt.Errorf("unable to get Docker info: %s", err)
	}
	return parseStorageStatsFromInfo(info)
}

func isImageShaOrRepoDigest(image string) bool {
	return strings.HasPrefix(image, "sha256:") || strings.Contains(image, "@sha256:")
}

// ResolveImageName will resolve sha image name to their user-friendly name.
// For non-sha/non-repodigest names we will just return the name as-is.
func (d *DockerUtil) ResolveImageName(ctx context.Context, image string) (string, error) {
	if !isImageShaOrRepoDigest(image) {
		return image, nil
	}

	d.Lock()
	if preferredName, found := d.imageNameBySha[image]; found {
		d.Unlock()
		return preferredName, nil
	}

	ctx, cancel := context.WithTimeout(ctx, d.queryTimeout)
	defer cancel()
	r, _, err := d.cli.ImageInspectWithRaw(ctx, image)
	if err != nil {
		// Only log errors that aren't "not found" because some images may
		// just not be available in docker inspect.
		if !client.IsErrNotFound(err) {
			d.Unlock()
			return image, err
		}
		d.imageNameBySha[image] = image
	}

	d.Unlock()
	return d.GetPreferredImageName(r.ID, r.RepoTags, r.RepoDigests), nil
}

// GetPreferredImageName returns preferred image name based on RepoTags and RepoDigests
func (d *DockerUtil) GetPreferredImageName(imageID string, repoTags []string, repoDigests []string) string {
	d.Lock()
	defer d.Unlock()

	if preferredName, found := d.imageNameBySha[imageID]; found {
		return preferredName
	}

	var preferredName string
	// Try RepoTags first and fall back to RepoDigest otherwise.
	if len(repoTags) > 0 {
		sort.Strings(repoTags)
		preferredName = repoTags[0]
	} else if len(repoDigests) > 0 {
		// Digests formatted like quay.io/foo/bar@sha256:hash
		sort.Strings(repoDigests)
		sp := strings.SplitN(repoDigests[0], "@", 2)
		preferredName = sp[0]
	} else {
		preferredName = imageID
	}

	d.imageNameBySha[imageID] = preferredName
	return preferredName
}

// ImageInspect returns an image inspect object for a given image ID
func (d *DockerUtil) ImageInspect(ctx context.Context, imageID string) (types.ImageInspect, error) {
	ctx, cancel := context.WithTimeout(ctx, d.queryTimeout)
	defer cancel()

	imageInspect, _, err := d.cli.ImageInspectWithRaw(ctx, imageID)
	if err != nil {
		return imageInspect, fmt.Errorf("error inspecting image: %w", err)
	}

	return imageInspect, nil
}

// ImageHistory returns the history for a given image ID
func (d *DockerUtil) ImageHistory(ctx context.Context, imageID string) ([]image.HistoryResponseItem, error) {
	ctx, cancel := context.WithTimeout(ctx, d.queryTimeout)
	defer cancel()

	history, err := d.cli.ImageHistory(ctx, imageID)
	if err != nil {
		return history, fmt.Errorf("error getting image history: %w", err)
	}

	return history, nil
}

// ResolveImageNameFromContainer will resolve the container sha image name to their user-friendly name.
// It is similar to ResolveImageName except it tries to match the image to the container Config.Image.
// For non-sha names we will just return the name as-is.
func (d *DockerUtil) ResolveImageNameFromContainer(ctx context.Context, co types.ContainerJSON) (string, error) {
	if co.Config.Image != "" && !isImageShaOrRepoDigest(co.Config.Image) {
		return co.Config.Image, nil
	}

	return d.ResolveImageName(ctx, co.Image)
}

// Inspect returns a docker inspect object for a given container ID.
// It tries to locate the container in the inspect cache before making the docker inspect call
func (d *DockerUtil) Inspect(ctx context.Context, id string, withSize bool) (types.ContainerJSON, error) {
	cacheKey := GetInspectCacheKey(id, withSize)
	var container types.ContainerJSON

	cached, hit := cache.Cache.Get(cacheKey)
	// Try to get sized hit if we got a miss and withSize=false
	if !hit && !withSize {
		cached, hit = cache.Cache.Get(GetInspectCacheKey(id, true))
	}

	if hit {
		container, ok := cached.(types.ContainerJSON)
		if !ok {
			log.Errorf("Invalid inspect cache format, forcing a cache miss")
		} else {
			return container, nil
		}
	}

	container, err := d.InspectNoCache(ctx, id, withSize)
	if err != nil {
		return container, err
	}

	// cache the inspect for 10 seconds to reduce pressure on the daemon
	cache.Cache.Set(cacheKey, container, 10*time.Second)

	return container, nil
}

// InspectNoCache returns a docker inspect object for a given container ID. It
// ignores the inspect cache, always collecting fresh data from the docker
// daemon.
func (d *DockerUtil) InspectNoCache(ctx context.Context, id string, withSize bool) (types.ContainerJSON, error) {
	ctx, cancel := context.WithTimeout(ctx, d.queryTimeout)
	defer cancel()

	container, _, err := d.cli.ContainerInspectWithRaw(ctx, id, withSize)
	if client.IsErrNotFound(err) {
		return container, dderrors.NewNotFound(fmt.Sprintf("docker container %s", id))
	}
	if err != nil {
		return container, err
	}

	// ContainerJSONBase is a pointer embed, so it might be nil and cause segfaults
	if container.ContainerJSONBase == nil {
		return container, errors.New("invalid inspect data")
	}

	return container, nil
}

// AllContainerLabels retrieves all running containers (`docker ps`) and returns
// a map mapping containerID to container labels as a map[string]string
func (d *DockerUtil) AllContainerLabels(ctx context.Context) (map[string]map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, d.queryTimeout)
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

// GetContainerStats returns docker container stats
func (d *DockerUtil) GetContainerStats(ctx context.Context, containerID string) (*types.StatsJSON, error) {
	ctx, cancel := context.WithTimeout(ctx, d.queryTimeout)
	defer cancel()
	stats, err := d.cli.ContainerStatsOneShot(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("unable to get Docker stats: %s", err)
	}
	containerStats := &types.StatsJSON{}
	err = json.NewDecoder(stats.Body).Decode(&containerStats)
	if err != nil {
		return nil, fmt.Errorf("error listing containers: %s", err)
	}
	return containerStats, nil
}

// ContainerLogs returns a container logs reader
func (d *DockerUtil) ContainerLogs(ctx context.Context, container string, options types.ContainerLogsOptions) (io.ReadCloser, error) {
	return d.cli.ContainerLogs(ctx, container, options)
}

// GetContainerPIDs returns a list of containerID's running PIDs
func (d *DockerUtil) GetContainerPIDs(ctx context.Context, containerID string) ([]int, error) {

	// Index into the returned [][]string slice for process IDs
	pidIdx := -1

	// Docker API to collect PIDs associated with containerID
	procs, err := d.cli.ContainerTop(ctx, containerID, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to get PIDs for container %s: %s", containerID, err)
	}

	// get the offset into the string[][] slice for the process ID index
	for idx, val := range procs.Titles {
		if val == "PID" {
			pidIdx = idx
			break
		}
	}
	if pidIdx == -1 {
		return nil, fmt.Errorf("unable to locate PID index into returned process slice")
	}

	// Create slice large enough to hold each PID
	pids := make([]int, len(procs.Processes))

	// Iterate returned Processes and pull out their PIDs
	for idx, entry := range procs.Processes {
		// Convert to ints
		pid, sterr := strconv.Atoi(entry[pidIdx])
		if sterr != nil {
			log.Debugf("unable to convert PID to int: %s", sterr)
			continue
		}
		pids[idx] = pid
	}
	return pids, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types"

	"github.com/StackVista/stackstate-agent/pkg/util/containers"
	"github.com/StackVista/stackstate-agent/pkg/util/containers/metrics"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

var healthRe = regexp.MustCompile(`\(health: (\w+)\)`)

// ContainerListConfig allows to pass listing options
type ContainerListConfig struct {
	IncludeExited bool
	FlagExcluded  bool
}

// Containers gets a list of all containers on the current node using a mix of
// the Docker APIs and cgroups stats. We attempt to limit syscalls where possible.
func (d *DockerUtil) ListContainers(cfg *ContainerListConfig) ([]*containers.Container, error) {
	cgByContainer, err := metrics.ScrapeAllCgroups()
	if err != nil {
		return nil, fmt.Errorf("could not get cgroups: %s", err)
	}

	cList, err := d.dockerContainers(cfg)
	if err != nil {
		return nil, fmt.Errorf("could not get docker containers: %s", err)
	}

	for _, container := range cList {
		if container.State != containers.ContainerRunningState || container.Excluded {
			continue
		}
		cgroup, ok := cgByContainer[container.ID]
		if !ok {
			log.Debugf("No matching cgroups for container %s, skipping", container.ID[:12])
			continue
		}
		container.SetCgroups(cgroup)
		err = container.FillCgroupLimits()
		if err != nil {
			log.Debugf("Cannot get limits for container %s: %s, skipping", container.ID[:12], err)
			continue
		}
	}
	err = d.UpdateContainerMetrics(cList)
	return cList, err
}

// UpdateContainerMetrics updates cgroup / network performance metrics for
// a provided list of Container objects
func (d *DockerUtil) UpdateContainerMetrics(cList []*containers.Container) error {
	for _, container := range cList {
		if container.State != containers.ContainerRunningState || container.Excluded {
			continue
		}

		err := container.FillCgroupMetrics()
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
	}
	return nil
}

// dockerContainers returns the running container list from the docker API
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
		if d.cfg.CollectNetwork && c.State == containers.ContainerRunningState {
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

		entityID := ContainerIDToEntityName(c.ID)
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
		d.cleanupCaches(cList)
	}

	return ret, nil
}

// Parse the health out of a container status. The format is either:
//  - 'Up 5 seconds (health: starting)'
//  - 'Up about an hour'
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

// cleanupCaches removes cache entries for unknown containers and images
func (d *DockerUtil) cleanupCaches(containers []types.Container) {
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

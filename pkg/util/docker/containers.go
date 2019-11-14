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
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

		if isMissingIP(container.AddressList) {
			hostIPs := GetDockerHostIPs()
			container.AddressList = correctMissingIPs(container.AddressList, hostIPs)
		} else if len(container.AddressList) == 0 {
			// the inspect should be in the cache already so this is not a problem
			inspect, err := d.Inspect(container.ID, false)
			if err != nil {
				log.Debugf("Error inspecting container %s: %s", container.ID, err)
				continue
			}
			// empty network settings or empty network can indicate
			// the container is in awsvpc networking mode so we try that
			if inspect.NetworkSettings == nil || len(inspect.NetworkSettings.Networks) == 0 {
				networkMode, err := GetContainerNetworkMode(container.ID)
				log.Tracef("container %s network mode: %s", container.Name, networkMode)
				if err != nil {
					log.Debugf("Failed to get network mode for container %s. Network info will be missing. Error: %s", container.ID, err)
					continue
				}
				if networkMode == containers.AwsvpcNetworkMode {
					ecsContainerMetadataUrl, err := d.getECSMetadataURL(container.ID)

					if err != nil {
						log.Debugf("Failed to get the ECS container metadata URI for container %s. Network info will be missing. Error: %s", container.ID, err)
						continue
					}

					res, err := metadata.GetContainerNetworkAddresses(ecsContainerMetadataUrl)

					if err != nil {
						log.Errorf("Failed to get container network addresses: %s", err)
						continue
					}
					container.AddressList = res
				}
			}
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

		entityID := ContainerIDToTaggerEntityName(c.ID)
		container := &containers.Container{
			Type:        "Docker",
			ID:          c.ID,
			EntityID:    entityID,
			Name:        c.Names[0],
			Image:       image,
			ImageID:     c.ImageID,
			Created:     c.Created,
			State:       c.State,
			Excluded:    excluded,
			Health:      parseContainerHealth(c.Status),
			AddressList: d.parseContainerNetworkAddresses(c.ID, c.Ports, c.NetworkSettings, c.Names[0]),
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
//  - 'Up 18 hours (unhealthy)'
//  - 'Up about an hour'
func parseContainerHealth(status string) string {
	// Avoid allocations in most cases by just checking for '('
	if strings.Index(status, "unhealthy") >= 0 {
		return "unhealthy"
	}
	if strings.IndexByte(status, '(') == -1 {
		return ""
	}
	all := healthRe.FindAllStringSubmatch(status, -1)
	if len(all) < 1 || len(all[0]) < 2 {
		return ""
	}
	return all[0][1]
}

// parseContainerNetworkAddresses converts docker ports
// and network settings into a list of NetworkAddress
func (d *DockerUtil) parseContainerNetworkAddresses(cID string, ports []types.Port, netSettings *types.SummaryNetworkSettings, container string) []containers.NetworkAddress {
	addrList := []containers.NetworkAddress{}
	tempAddrList := []containers.NetworkAddress{}
	if netSettings == nil || len(netSettings.Networks) == 0 {
		log.Debugf("No network settings available from docker")
		return addrList
	}
	for _, port := range ports {
		if isExposed(port) {
			IP := net.ParseIP(port.IP)
			if IP == nil {
				log.Warnf("Unable to parse IP: %v for container: %s", port.IP, container)
				continue
			}
			addrList = append(addrList, containers.NetworkAddress{
				IP:       IP,                   // Host IP, since the port is exposed
				Port:     int(port.PublicPort), // Exposed port
				Protocol: port.Type,
			})
		}
		// Cache container ports
		tempAddrList = append(tempAddrList, containers.NetworkAddress{
			Port:     int(port.PrivatePort),
			Protocol: port.Type,
		})
	}
	// Retieve IPs from network settings for the cached ports
	for _, network := range netSettings.Networks {
		if network.IPAddress == "" {
			log.Debugf("No IP found for container %s in network %s", container, network.NetworkID)
			continue
		}
		IP := net.ParseIP(network.IPAddress)
		if IP == nil {
			log.Warnf("Unable to parse IP: %v for container: %s", network.IPAddress, container)
			continue
		}
		for _, addr := range tempAddrList {
			// Add IP to the cached and not exposed ports
			addrList = append(addrList, containers.NetworkAddress{
				IP:       IP,
				Port:     int(addr.Port),
				Protocol: addr.Protocol,
			})
		}
	}
	return addrList
}

// isExposed returns if a docker port is exposed to the host
func isExposed(port types.Port) bool {
	return port.PublicPort > 0 && port.IP != ""
}

// getECSMetadataURL inspects a given container ID and returns its ECS container metadata URI
// if found in its environment. It returns an empty string and an error on failure.
func (d *DockerUtil) getECSMetadataURL(cID string) (string, error) {
	i, err := d.Inspect(cID, false)
	if err != nil {
		return "", err
	}
	for _, e := range i.Config.Env {
		if strings.HasPrefix(e, "ECS_CONTAINER_METADATA_URI=") {
			return strings.Split(e, "=")[1], nil
		}
	}
	return "", errors.New("ecs container metadata uri not found")
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

var missingIP = net.ParseIP("0.0.0.0")

func isMissingIP(addrs []containers.NetworkAddress) bool {
	for _, addr := range addrs {
		if addr.IP.Equal(missingIP) {
			return true
		}
	}
	return false
}

func correctMissingIPs(addrs []containers.NetworkAddress, hostIPs []string) []containers.NetworkAddress {
	if len(hostIPs) == 0 {
		return addrs // cannot detect host list, will return the addresses as is
	}

	var correctedAddrs []containers.NetworkAddress

	for _, addr := range addrs {
		if addr.IP.Equal(missingIP) {
			for _, hip := range hostIPs {
				correctedAddr := addr // this will copy addr
				correctedAddr.IP = net.ParseIP(hip)
				correctedAddrs = append(correctedAddrs, correctedAddr)
			}
		} else {
			correctedAddrs = append(correctedAddrs, addr)
		}
	}
	return correctedAddrs
}

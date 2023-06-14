// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"encoding/binary"
	"net"
	"runtime"
	"strings"
	"time"

	dockerTypes "github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/config"
	taggerUtils "github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	containerModePrefix = "container:"
)

// The custom network extension is only there to build the interface name to Docker network name mapping
// It only works on Linux with access to host /proc, which should not be a pre-requisite.
func (d *DockerCheck) configureNetworkProcessor(processor *generic.Processor) {
	switch runtime.GOOS {
	case "linux":
		if config.IsHostProcAvailable() {
			d.networkProcessorExtension = &dockerNetworkExtension{procPath: config.Datadog.GetString("container_proc_root")}
		}
	case "windows":
		d.networkProcessorExtension = &dockerNetworkExtension{}
	default:
		panic("Not implemented")
	}

	if d.networkProcessorExtension != nil {
		processor.RegisterExtension(generic.NetworkExtensionID, d.networkProcessorExtension)
	}
}

type containerNetworkEntry struct {
	containerID  string
	tags         []string
	pids         []int
	networkStats *metrics.ContainerNetworkStats

	ifaceNetworkMapping map[string]string
	networkContainerID  string
}

type dockerNetworkExtension struct {
	procPath                string
	containerNetworkEntries map[string]*containerNetworkEntry
	sender                  generic.SenderFunc
	aggSender               aggregator.Sender
}

// PreProcess creates a new empty mapping for the upcoming check run
func (dn *dockerNetworkExtension) PreProcess(sender generic.SenderFunc, aggSender aggregator.Sender) {
	dn.sender = sender
	dn.aggSender = aggSender
	dn.containerNetworkEntries = make(map[string]*containerNetworkEntry)
}

// Process is called after core process (regardless of encountered error)
func (dn *dockerNetworkExtension) Process(tags []string, container *workloadmeta.Container, collector metrics.Collector, cacheValidity time.Duration) {
	// Duplicate call with generic.Processor, but cache should allow for a fast response.
	// We only need it for PIDs
	containerStats, err := collector.GetContainerStats(container.Namespace, container.ID, cacheValidity)
	if err != nil {
		log.Debugf("Gathering container metrics for container: %v failed, metrics may be missing, err: %v", container, err)
		return
	}

	if containerStats == nil {
		log.Debugf("Metrics provider returned nil stats for container: %v", container)
		return
	}

	containerNetworkStats, err := collector.GetContainerNetworkStats(container.Namespace, container.ID, cacheValidity)
	if err != nil {
		log.Debugf("Gathering network metrics for container: %v failed, metrics may be missing, err: %v", container, err)
		return
	}

	if containerNetworkStats == nil {
		log.Debugf("Metrics provider returned nil network stats for container: %v", container)
		return
	}

	containerEntry := &containerNetworkEntry{
		containerID:  container.ID,
		networkStats: containerNetworkStats,
		tags:         tags,
	}

	if containerStats.PID != nil {
		containerEntry.pids = containerStats.PID.PIDs
	}

	dn.containerNetworkEntries[container.ID] = containerEntry
}

// PostProcess is called once during each check run, after all calls to `Process`
func (dn *dockerNetworkExtension) PostProcess() {
	// Nothing to do here
}

// Custom interface linked to Docker check interaction
func (dn *dockerNetworkExtension) preRun() {
	// Nothing to do here
}

func (dn *dockerNetworkExtension) processContainer(rawContainer dockerTypes.Container) {
	// If containerNetworkEntries is nil, it means the generic check was not able to run properly.
	// It's then useless to run.
	if dn.containerNetworkEntries == nil {
		return
	}

	// We keep excluded containers because pause containers are required as they usually hold
	// the network configuration for other containers.
	// However stopped containers are not useful there.
	if rawContainer.State != string(workloadmeta.ContainerStatusRunning) {
		return
	}

	containerEntry := dn.containerNetworkEntries[rawContainer.ID]
	if containerEntry == nil {
		containerEntry = &containerNetworkEntry{
			containerID: rawContainer.ID,
		}
		dn.containerNetworkEntries[rawContainer.ID] = containerEntry
	}

	findDockerNetworks(dn.procPath, containerEntry, rawContainer)
}

func (dn *dockerNetworkExtension) postRun() {
	// If containerNetworkEntries is nil, it means the generic check was not able to run properly.
	// It's then useless to run.
	if dn.containerNetworkEntries == nil {
		return
	}

	for _, containerEntry := range dn.containerNetworkEntries {
		// This is expected as we store excluded containers (like pause containers), created when processing `rawContainer`.
		// If there was a real failure when gathering NetworkStats, a debug is emitted in `Process()`
		if containerEntry.networkStats == nil {
			continue
		}

		// For the Docker check, if we don't have per interface and a interface-network mapping, we just don't report
		if len(containerEntry.networkStats.Interfaces) == 0 {
			log.Debugf("Empty network metrics for container %s", containerEntry.containerID)
			continue
		}

		// Stats are always taken from current `containerEntry` but iface<>docker network name mapping
		// may be taken from another containerNetworkEntry, named networkMappingEntry.
		// It typically happens in Kubernetes with the pause container.
		networkMappingEntry := containerEntry
		if containerEntry.networkContainerID != "" {
			if entry, found := dn.containerNetworkEntries[containerEntry.networkContainerID]; found {
				networkMappingEntry = entry
			}
		}

		for interfaceName, interfaceStats := range containerEntry.networkStats.Interfaces {
			if mappedName, found := networkMappingEntry.ifaceNetworkMapping[interfaceName]; found {
				interfaceName = mappedName
			}

			interfaceTags := taggerUtils.ConcatenateStringTags(containerEntry.tags, "docker_network:"+interfaceName)

			dn.sender(dn.aggSender.Rate, "docker.net.bytes_sent", interfaceStats.BytesSent, interfaceTags)
			dn.sender(dn.aggSender.Rate, "docker.net.bytes_rcvd", interfaceStats.BytesRcvd, interfaceTags)
		}
	}
}

// Allow mocking in unit tests
var (
	getRoutesFunc = system.ParseProcessRoutes
)

func findDockerNetworks(procPath string, entry *containerNetworkEntry, container dockerTypes.Container) {
	netMode := container.HostConfig.NetworkMode
	// Check the known network modes that require specific handling.
	// Other network modes will look at the docker NetworkSettings.
	if netMode == docker.HostNetworkMode {
		log.Debugf("Container %s is in network host mode, its network metrics are for the whole host", entry.containerID)
		return
	} else if netMode == docker.NoneNetworkMode {
		// Keep legacy behavior, maping eth0 to bridge
		entry.ifaceNetworkMapping = map[string]string{"eth0": "bridge"}
		return
	} else if strings.HasPrefix(netMode, containerModePrefix) {
		entry.networkContainerID = strings.TrimPrefix(netMode, containerModePrefix)
		return
	}

	// Verify that we aren't using an older version of Docker that does
	// not provide the network settings in container inspect.
	netSettings := container.NetworkSettings
	if netSettings == nil || netSettings.Networks == nil || len(netSettings.Networks) == 0 {
		log.Debugf("No network settings available from docker, defaulting to host network")
		return
	}

	// We need at least one PID to gather routes
	if len(entry.pids) == 0 {
		log.Debugf("No PID found for container: %s, skipping network", entry.containerID)
		return
	}

	var err error
	interfaces := make(map[string]uint64)
	for netName, netConf := range netSettings.Networks {
		if netName == "host" {
			log.Debugf("Container %s is in network host mode, its network metrics are for the whole host", entry.containerID)
			return
		}

		ipString := netConf.IPAddress
		// Check if this is a CIDR or just an IP
		var ip net.IP
		if strings.Contains(ipString, "/") {
			ip, _, err = net.ParseCIDR(ipString)
			if err != nil {
				log.Warnf("Malformed IP %s for container id %s: %s, skipping", ipString, entry.containerID, err)
				continue
			}
		} else {
			ip = net.ParseIP(ipString)
			if ip == nil {
				log.Warnf("Malformed IP %s for container id %s: %s, skipping", ipString, entry.containerID, err)
				continue
			}
		}

		// Convert IP to little endian uint64 for comparison to network routes.
		interfaces[netName] = uint64(binary.LittleEndian.Uint32(ip.To4()))
	}

	destinations, err := getRoutesFunc(procPath, entry.pids[0])
	if err != nil {
		log.Warnf("Cannot list routes for container id %s: %s, skipping", entry.containerID, err)
		return
	}

	entry.ifaceNetworkMapping = make(map[string]string, len(destinations))
	for _, d := range destinations {
		for n, ip := range interfaces {
			if d.Subnet != 0 && ip&d.Mask == d.Subnet {
				entry.ifaceNetworkMapping[d.Interface] = n
			}
		}
	}
}

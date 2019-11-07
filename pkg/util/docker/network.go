// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type dockerNetwork struct {
	iface      string
	dockerName string
	// Temporary store of id for containers that route through another container
	// such as in the "pod container" case used by Kubernetes. The network
	// resolution should resolve this network to the correct interface from the
	// referenced container.
	routingContainerID string
}

type dockerNetworks []dockerNetwork

func (a dockerNetworks) Len() int           { return len(a) }
func (a dockerNetworks) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a dockerNetworks) Less(i, j int) bool { return a[i].dockerName < a[j].dockerName }

var hostNetwork = dockerNetwork{iface: "eth0", dockerName: "bridge"}

const (
	ecsPauseContainerImage = "amazon/amazon-ecs-pause"
	containerModePrefix    = "container:"
)

func findDockerNetworks(containerID string, pid int, container types.Container) []dockerNetwork {
	netMode := container.HostConfig.NetworkMode
	// Check the known network modes that require specific handling.
	// Other network modes will look at the docker NetworkSettings.
	if netMode == containers.HostNetworkMode {
		log.Debugf("Container %s is in network host mode, its network metrics are for the whole host", containerID)
		return []dockerNetwork{hostNetwork}
	} else if netMode == containers.NoneNetworkMode {
		log.Debugf("Container %s is in network mode 'none', we will collect metrics for the whole host", containerID)
		return []dockerNetwork{hostNetwork}
	} else if strings.HasPrefix(netMode, "container:") {
		netContainerID := strings.TrimPrefix(netMode, "container:")
		log.Debugf("Container %s uses the network namespace of container:%s", containerID, netContainerID)
		return []dockerNetwork{{routingContainerID: netContainerID}}
	}

	// Verify that we aren't using an older version of Docker that does
	// not provide the network settings in container inspect.
	netSettings := container.NetworkSettings
	if netSettings == nil || netSettings.Networks == nil || len(netSettings.Networks) == 0 {
		log.Debugf("No network settings available from docker, defaulting to host network")
		return []dockerNetwork{hostNetwork}
	}

	var err error
	interfaces := make(map[string]uint64)
	for netName, netConf := range netSettings.Networks {
		if netName == "host" {
			log.Debugf("Container %s is in network host mode, its network metrics are for the whole host", containerID)
			return []dockerNetwork{hostNetwork}
		}

		ipString := netConf.IPAddress
		// Check if this is a CIDR or just an IP
		var ip net.IP
		if strings.Contains(ipString, "/") {
			ip, _, err = net.ParseCIDR(ipString)
			if err != nil {
				log.Warnf("Malformed IP %s for container id %s: %s, skipping", ipString, containerID, err)
				continue
			}
		} else {
			ip = net.ParseIP(ipString)
			if ip == nil {
				log.Warnf("Malformed IP %s for container id %s: %s, skipping", ipString, containerID, err)
				continue
			}
		}

		// Convert IP to little endian uint64 for comparison to network routes.
		interfaces[netName] = uint64(binary.LittleEndian.Uint32(ip.To4()))
	}

	destinations, err := metrics.DetectNetworkDestinations(pid)
	if err != nil {
		log.Warnf("Cannot list interfaces for container id %s: %s, skipping", containerID, err)
		return nil
	}

	networks := make([]dockerNetwork, 0)
	for _, d := range destinations {
		for n, ip := range interfaces {
			if ip&d.Mask == d.Subnet {
				networks = append(networks, dockerNetwork{iface: d.Interface, dockerName: n})
			}
		}
	}
	sort.Sort(dockerNetworks(networks))
	return networks
}

// resolveDockerNetworks will resolve any network mappings in-place for any
// networks that are pointing to a containerID and rely on another containers
// network namespace. All other networks are left as-is.
// This should be called after findDockerNetworks is called for all running
// containers.
func resolveDockerNetworks(containerNetworks map[string][]dockerNetwork) {
	for cid, networks := range containerNetworks {
		for _, nw := range networks {
			if nw.routingContainerID == "" {
				continue
			}
			if cnw, ok := containerNetworks[nw.routingContainerID]; ok {
				containerNetworks[cid] = cnw
			} else {
				log.Debugf("unable to resolve network for c:%s that uses namespace of c:%s", cid, nw.routingContainerID)
				containerNetworks[cid] = nil
			}
		}
	}
}

// DefaultGateway returns the default Docker gateway.
func DefaultGateway() (net.IP, error) {
	fields, err := defaultGatewayFields()
	if err != nil || len(fields) < 3 {
		return nil, err
	}

	ipInt, err := strconv.ParseUint(fields[2], 16, 32)
	if err != nil {
		return nil, fmt.Errorf("unable to parse ip %s from route file: %s", fields[2], err)
	}
	ip := make(net.IP, 4)
	binary.LittleEndian.PutUint32(ip, uint32(ipInt))
	return ip, nil
}

// DefaultHostIPs returns the IP addresses bound to the default network interface.
// The default network interface is the one connected to the network gateway, and it is determined
// by parsing the routing table file in the proc file system.
func DefaultHostIPs() ([]string, error) {
	fields, err := defaultGatewayFields()
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("missing interface information from routing file")
	}
	iface, err := net.InterfaceByName(fields[0])
	if err != nil {
		return nil, err
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}

	ips := make([]string, len(addrs))
	for i, addr := range addrs {
		// Translate CIDR blocks into IPs, if necessary
		ips[i] = strings.Split(addr.String(), "/")[0]
	}

	return ips, nil
}

// defaultGatewayFields extracts the fields associated to the interface connected
// to the network gateway from the linux routing table. As an example, for the given file in /proc/net/routes:
//
// Iface   Destination  Gateway   Flags  RefCnt  Use  Metric  Mask      MTU  Window  IRTT
// enp0s3  00000000     0202000A  0003   0       0    0       00000000  0    0       0
// enp0s3  0002000A     00000000  0001   0       0    0       00FFFFFF  0    0       0
//
// The returned value would be ["enp0s3","00000000","0202000A","0003","0","0","0","00000000","0","0","0"]
//
func defaultGatewayFields() ([]string, error) {
	procRoot := config.Datadog.GetString("proc_root")
	netRouteFile := filepath.Join(procRoot, "net", "route")
	f, err := os.Open(netRouteFile)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			log.Errorf("unable to open %s: %s", netRouteFile, err)
			return nil, nil
		}
		// Unknown error types will bubble up for handling.
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 1 && fields[1] == "00000000" {
			return fields, nil
		}
	}

	return nil, fmt.Errorf("couldn't retrieve default gateway information")
}

// GetAgentContainerNetworkMode provides the network mode of the Agent container
// To get this info in an optimal way, consider calling util.GetAgentNetworkMode
// instead to benefit from the cache
func GetAgentContainerNetworkMode() (string, error) {
	du, err := GetDockerUtil()
	if err != nil {
		return "", err
	}
	agentContainer, err := du.InspectSelf()
	if err != nil {
		return "", err
	}
	mode, err := parseContainerNetworkMode(agentContainer.HostConfig)
	if err != nil {
		return mode, err
	}

	// Try to discover awsvpc mode
	if strings.HasPrefix(mode, containerModePrefix) {
		// Inspect the attached container
		co, err := du.Inspect(mode[len(containerModePrefix):], false)
		if err != nil {
			return "", fmt.Errorf("cannot inspect attached container %s: %v", mode, err)
		}
		// In awsvpc mode, the attached container is an amazon ecs pause container
		if co.Config != nil && strings.HasPrefix(co.Config.Image, ecsPauseContainerImage) {
			return containers.AwsvpcNetworkMode, nil
		} else {
			return containers.UnknownNetworkMode, fmt.Errorf("unknown network mode: %s", mode)
		}
	}
	return mode, nil
}

// parseContainerNetworkMode returns the network mode of a container
func parseContainerNetworkMode(hostConfig *container.HostConfig) (string, error) {
	if hostConfig == nil {
		return "", errors.New("the HostConfig field is nil")
	}
	mode := string(hostConfig.NetworkMode)
	switch mode {
	case containers.DefaultNetworkMode:
		return containers.DefaultNetworkMode, nil
	case containers.HostNetworkMode:
		return containers.HostNetworkMode, nil
	case containers.BridgeNetworkMode:
		return containers.BridgeNetworkMode, nil
	case containers.NoneNetworkMode:
		return containers.NoneNetworkMode, nil
	}
	if strings.HasPrefix(mode, containerModePrefix) {
		return mode, nil
	}
	return containers.UnknownNetworkMode, fmt.Errorf("unknown network mode: %s", mode)
}

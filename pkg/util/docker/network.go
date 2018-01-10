// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/config"
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

func findDockerNetworks(containerID string, pid int, container types.Container) []dockerNetwork {
	netMode := container.HostConfig.NetworkMode
	// Check the known network modes that require specific handling.
	// Other network modes will look at the docker NetworkSettings.
	if netMode == "host" {
		log.Debugf("Container %s is in network host mode, its network metrics are for the whole host", containerID)
		return []dockerNetwork{hostNetwork}
	} else if netMode == "none" {
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
	dockerGateways := make(map[string]int64)
	for netName, netConf := range netSettings.Networks {
		gw := netConf.Gateway
		if netName == "host" || gw == "" {
			log.Debugf("Empty network gateway, container %s is in network host mode, its network metrics are for the whole host", containerID)
			return []dockerNetwork{hostNetwork}
		}

		// Check if this is a CIDR or just an IP
		var ip net.IP
		if strings.Contains(gw, "/") {
			ip, _, err = net.ParseCIDR(gw)
			if err != nil {
				log.Warnf("Invalid gateway %s for container id %s: %s, skipping", gw, containerID, err)
				continue
			}
		} else {
			ip = net.ParseIP(gw)
			if ip == nil {
				log.Warnf("Invalid gateway %s for container id %s: %s, skipping", gw, containerID, err)
				continue
			}
		}

		// Convert IP to little endian int64 for comparison to network routes.
		dockerGateways[netName] = int64(binary.LittleEndian.Uint32(ip.To4()))
	}

	// Read contents of file. Handle missing or unreadable file in case container was stopped.
	procNetFile := hostProc(strconv.Itoa(int(pid)), "net", "route")
	if !pathExists(procNetFile) {
		log.Debugf("Missing %s for container %s", procNetFile, containerID)
		return nil
	}
	lines, err := readLines(procNetFile)
	if err != nil {
		log.Debugf("Unable to read %s for container %s", procNetFile, containerID)
		return nil
	}
	if len(lines) < 1 {
		log.Errorf("empty network file, unable to get docker networks: %s", procNetFile)
		return nil
	}

	networks := make([]dockerNetwork, 0)
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		if fields[1] == "00000000" {
			continue
		}
		dest, _ := strconv.ParseInt(fields[1], 16, 32)
		mask, _ := strconv.ParseInt(fields[7], 16, 32)
		for net, gw := range dockerGateways {
			if gw&mask == dest {
				networks = append(networks, dockerNetwork{iface: fields[0], dockerName: net})
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

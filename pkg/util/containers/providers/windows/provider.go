// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inmetrics.

// +build windows
// +build docker

package windows

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/winutil/iphelper"
	"github.com/docker/docker/pkg/sysinfo"
	"golang.org/x/sys/windows"

	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/containers/providers"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type containerBundle struct {
	metrics        *metrics.ContainerMetrics
	networkMetrics map[string]types.NetworkStats
	limits         *metrics.ContainerLimits
	startTime      int64
}

// Provider is a Windows implementation of the ContainerImplementation interface
type provider struct {
	containers     map[string]containerBundle
	agentCID       *string
	containersLock sync.RWMutex
	prefetchLock   sync.Mutex
}

func init() {
	providers.Register(&provider{})
}

// Prefetch gets data from all cgroups in one go
// If not successful all other calls will fail
func (mp *provider) Prefetch() error {
	// Prefetch() can be slow and we don't want to lock readers during all this time.
	// Also, we don't want multiple Prefetch() at the same time, so using 2 locks.
	mp.prefetchLock.Lock()
	defer mp.prefetchLock.Unlock()

	dockerUtil, err := docker.GetDockerUtil()
	if err != nil {
		return err
	}

	// We don't need exited/stopped containers
	rawContainers, err := dockerUtil.RawContainerList(types.ContainerListOptions{})
	if err != nil {
		return err
	}

	// Used to find if Agent is running in a container.
	// With K8S entrypoint, `agentPID` should match
	// With Docker entrypoint, `parentPID` should match
	agentPID := os.Getpid()
	parentPID := os.Getppid()

	containers := make(map[string]containerBundle, len(rawContainers))
	for _, container := range rawContainers {
		containerBundle := containerBundle{}

		cjson, err := dockerUtil.Inspect(container.ID, false)
		if err == nil {
			mp.fillContainerDetails(cjson, &containerBundle)

			// Luckily for us, on Windows PIDs are the same inside/outside containers
			if cjson.State.Pid == agentPID || cjson.State.Pid == parentPID {
				mp.agentCID = &container.ID
			}
		} else {
			log.Debugf("Impossible to inspect container %s: %v", container.ID, err)
		}

		stats, err := dockerUtil.GetContainerStats(container.ID)
		if err == nil && stats != nil {
			mp.fillContainerMetrics(stats, &containerBundle)
			mp.fillContainerNetworkMetrics(stats, &containerBundle)
		} else {
			log.Debugf("Impossible to get stats for container %s: %v", container.ID, err)
		}

		containers[container.ID] = containerBundle
	}

	mp.containersLock.Lock()
	defer mp.containersLock.Unlock()
	mp.containers = containers

	return nil
}

func (mp *provider) fillContainerDetails(cjson types.ContainerJSON, containerBundle *containerBundle) {
	// Parsing start time
	t, err := time.Parse(time.RFC3339, cjson.State.StartedAt)
	if err == nil {
		containerBundle.startTime = t.Unix()
	} else {
		log.Debugf("Impossible to get start time for container %s: %v", cjson.ID, err)
	}

	// Parsing limits
	var cpuMax float64 = 0
	if cjson.HostConfig.NanoCPUs > 0 {
		cpuMax = float64(cjson.HostConfig.NanoCPUs) / 1e9 / float64(sysinfo.NumCPU()) * 100
	} else if cjson.HostConfig.CPUPercent > 0 {
		cpuMax = float64(cjson.HostConfig.CPUPercent)
	} else if cjson.HostConfig.CPUCount > 0 {
		cpuMax = math.Min(float64(cjson.HostConfig.CPUCount), float64(sysinfo.NumCPU())) / float64(sysinfo.NumCPU()) * 100
	}
	containerBundle.limits = &metrics.ContainerLimits{
		CPULimit: cpuMax,
		MemLimit: uint64(cjson.HostConfig.Memory),
		//ThreadLimit: 0, // Unknown ?
	}
}

func (mp *provider) fillContainerMetrics(stats *types.StatsJSON, containerBundle *containerBundle) {
	// 100's of nanoseconds to jiffy
	kernel := stats.CPUStats.CPUUsage.UsageInKernelmode / 1e5
	total := stats.CPUStats.CPUUsage.TotalUsage / 1e5
	user := total - kernel
	if user < 0 {
		user = 0
	}

	containerBundle.metrics = &metrics.ContainerMetrics{
		CPU: &metrics.ContainerCPUStats{
			User:       user,
			System:     kernel,
			UsageTotal: float64(total),
		},
		Memory: &metrics.ContainerMemStats{
			// Send private working set as RSS even if it does not exactly match
			// since most dashboards expect this metric to be present
			RSS:               stats.MemoryStats.PrivateWorkingSet,
			PrivateWorkingSet: stats.MemoryStats.PrivateWorkingSet,
			CommitBytes:       stats.MemoryStats.Commit,
			CommitPeakBytes:   stats.MemoryStats.CommitPeak,
		},
		IO: &metrics.ContainerIOStats{
			ReadBytes:  stats.StorageStats.ReadSizeBytes,
			WriteBytes: stats.StorageStats.WriteSizeBytes,
		},
	}
}

func (mp *provider) fillContainerNetworkMetrics(stats *types.StatsJSON, containerBundle *containerBundle) {
	containerBundle.networkMetrics = stats.Networks
}

// ContainerExists returns true if a cgroup exists for this containerID
func (mp *provider) ContainerExists(containerID string) bool {
	mp.containersLock.RLock()
	defer mp.containersLock.RUnlock()

	_, exists := mp.containers[containerID]
	return exists
}

// GetContainerStartTime returns container start time
func (mp *provider) GetContainerStartTime(containerID string) (int64, error) {
	mp.containersLock.RLock()
	defer mp.containersLock.RUnlock()

	containerBundle, exists := mp.containers[containerID]
	if !exists {
		return 0, fmt.Errorf("container not found")
	}

	return containerBundle.startTime, nil
}

// GetContainerMetrics returns CPU, IO and Memory metrics
func (mp *provider) GetContainerMetrics(containerID string) (*metrics.ContainerMetrics, error) {
	mp.containersLock.RLock()
	defer mp.containersLock.RUnlock()

	containerBundle, exists := mp.containers[containerID]
	if !exists {
		return nil, fmt.Errorf("container not found")
	}

	return containerBundle.metrics, nil
}

// GetContainerLimits returns CPU, Thread and Memory limits
func (mp *provider) GetContainerLimits(containerID string) (*metrics.ContainerLimits, error) {
	mp.containersLock.RLock()
	defer mp.containersLock.RUnlock()

	containerBundle, exists := mp.containers[containerID]
	if !exists {
		return nil, fmt.Errorf("container not found")
	}

	return containerBundle.limits, nil
}

// GetNetworkMetrics return network metrics for all PIDs in container
func (mp *provider) GetNetworkMetrics(containerID string, networks map[string]string) (metrics.ContainerNetStats, error) {
	mp.containersLock.RLock()
	defer mp.containersLock.RUnlock()

	containerBundle, exists := mp.containers[containerID]
	if !exists {
		return nil, fmt.Errorf("container not found")
	}

	netStats := metrics.ContainerNetStats{}
	for ifaceName, netStat := range containerBundle.networkMetrics {
		var stat *metrics.InterfaceNetStats
		if nw, ok := networks[ifaceName]; ok {
			stat = &metrics.InterfaceNetStats{NetworkName: nw}
		} else {
			stat = &metrics.InterfaceNetStats{NetworkName: ifaceName}
		}
		stat.BytesRcvd = netStat.RxBytes
		stat.BytesSent = netStat.TxBytes
		stat.PacketsRcvd = netStat.RxPackets
		stat.PacketsSent = netStat.TxPackets

		netStats = append(netStats, stat)
	}
	return netStats, nil
}

// GetAgentCID returns the container ID where the current agent is running
func (mp *provider) GetAgentCID() (string, error) {
	// GetAgentCID is working without Prefetch() on Linux
	// Here we need Prefetch() to have run at least once
	if mp.agentCID == nil {
		log.Infof("AgentCID is empty, forcing a prefetch")
		mp.Prefetch()
	}

	// In case Prefetch() failed
	if mp.agentCID == nil {
		return "", nil
	}

	return *mp.agentCID, nil
}

// GetPIDs returns all PIDs running in the current container
func (mp *provider) GetPIDs(containerID string) ([]int32, error) {
	// FIXME: Figure out how to list PIDs from containers on Windows
	return nil, nil
}

// ContainerIDForPID return ContainerID for a given pid
func (mp *provider) ContainerIDForPID(pid int) (string, error) {
	// FIXME: Figure out how to list PIDs from containers on Windows
	return "", fmt.Errorf("not supported on windows")
}

// DetectNetworkDestinations lists all the networks available
// to a given PID and parses them in NetworkInterface objects
func (mp *provider) DetectNetworkDestinations(pid int) ([]containers.NetworkDestination, error) {
	// TODO: Filter by PID
	routingTable, err := iphelper.GetIPv4RouteTable()
	if err != nil {
		return nil, err
	}
	interfaceTable, err := iphelper.GetIFTable()
	if err != nil {
		return nil, err
	}
	netDestinations := make([]containers.NetworkDestination, 0)
	for _, row := range routingTable {
		itf := interfaceTable[row.DwForwardIfIndex]
		netDest := containers.NetworkDestination{
			Interface: windows.UTF16ToString(itf.WszName[:]),
			Subnet:    uint64(row.DwForwardDest),
			Mask:      uint64(row.DwForwardMask),
		}
		netDestinations = append(netDestinations, netDest)
	}
	return netDestinations, nil
}

// GetDefaultGateway returns the default gateway used by container implementation
func (mp *provider) GetDefaultGateway() (net.IP, error) {
	fields, err := defaultGatewayFields()
	if err != nil {
		return nil, err
	}
	return net.ParseIP(fields[2]), nil
}

// GetDefaultHostIPs returns the IP addresses bound to the default network interface.
// The default network interface is the one connected to the network gateway.
func (mp *provider) GetDefaultHostIPs() ([]string, error) {
	fields, err := defaultGatewayFields()
	if err != nil {
		return nil, err
	}
	//
	return []string{fields[3]}, nil
}

// Output from route print 0.0.0.0:
//
// λ route print 0.0.0.0
//===========================================================================
//Interface List
// 17...00 1c 42 86 10 92 ......Intel(R) 82574L Gigabit Network Connection
// 16...bc 9a 78 56 34 12 ......Bluetooth Device (Personal Area Network)
//  1...........................Software Loopback Interface 1
// 24...00 15 5d 2c 6f c0 ......Hyper-V Virtual Ethernet Adapter #2
//===========================================================================
//
//IPv4 Route Table
//===========================================================================
//Active Routes:
//Network Destination        Netmask          Gateway       Interface  Metric
//          0.0.0.0          0.0.0.0      10.211.55.1      10.211.55.4     25
//===========================================================================
//Persistent Routes:
//  Network Address          Netmask  Gateway Address  Metric
//          0.0.0.0          0.0.0.0      172.21.96.1  Default
//===========================================================================
//
//IPv6 Route Table
//===========================================================================
//Active Routes:
//  None
//Persistent Routes:
//  None
//
// We are interested in the Gateway and Interface fields of the Active Routes,
// so this method returns any line that has 5 fields with the first one being
// 0.0.0.0
func defaultGatewayFields() ([]string, error) {
	routeCmd := exec.Command("route", "print", "0.0.0.0")
	routeCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := routeCmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 5 && fields[0] == "0.0.0.0" {
			return fields, nil
		}
	}
	return nil, fmt.Errorf("couldn't retrieve default gateway information")
}

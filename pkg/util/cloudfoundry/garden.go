package cloudfoundry

import (
	"fmt"
	"net"
	"sync"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/client"
	"code.cloudfoundry.org/garden/client/connection"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/providers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	// ContainerNameTagKey tag key for container tags
	ContainerNameTagKey = "container_name"
	// AppInstanceGUIDTagKey tag key for container tags
	AppInstanceGUIDTagKey = "app_instance_guid"
	// AppNameTagKey tag key for container tags
	AppNameTagKey = "app_name"
	// AppInstanceIndexTagKey tag key for container tags
	AppInstanceIndexTagKey = "app_instance_index"
	// AppGUIDTagKey tag key for container tags
	AppGUIDTagKey = "app_guid"
)

var (
	globalGardenUtil     *GardenUtil
	globalGardenUtilLock sync.Mutex
)

// GardenUtilInterface describes a wrapper for collecting local garden containers
type GardenUtilInterface interface {
	GetGardenContainers() ([]garden.Container, error)
	ListContainers() ([]*containers.Container, error)
	UpdateContainerMetrics(cList []*containers.Container) error
}

// GardenUtil wraps interactions with a local garden API.
type GardenUtil struct {
	retrier retry.Retrier
	cli     client.Client
}

// GetGardenUtil returns the global instance of the garden utility and initializes it first if needed
func GetGardenUtil() (*GardenUtil, error) {
	globalGardenUtilLock.Lock()
	defer globalGardenUtilLock.Unlock()
	network := config.Datadog.GetString("cloud_foundry_garden.listen_network")
	address := config.Datadog.GetString("cloud_foundry_garden.listen_address")
	if globalGardenUtil == nil {
		globalGardenUtil = &GardenUtil{
			cli: client.New(connection.New(network, address)),
		}
		globalGardenUtil.retrier.SetupRetrier(&retry.Config{ //nolint:errcheck
			Name:          "gardenUtil",
			AttemptMethod: globalGardenUtil.cli.Ping,
			Strategy:      retry.RetryCount,
			RetryCount:    10,
			RetryDelay:    30 * time.Second,
		})
	}
	if err := globalGardenUtil.retrier.TriggerRetry(); err != nil {
		log.Debugf("Could not initiate connection to garden server %s using network %s: %s", address, network, err)
		return nil, err
	}
	return globalGardenUtil, nil
}

// GetGardenContainers returns the list of running containers from the local garden API
func (gu *GardenUtil) GetGardenContainers() ([]garden.Container, error) {
	return gu.cli.Containers(nil)
}

// ListContainers returns the list of running containers and populates their metrics and metadata
func (gu *GardenUtil) ListContainers() ([]*containers.Container, error) {
	if err := providers.ContainerImpl().Prefetch(); err != nil {
		return nil, fmt.Errorf("could not fetch container metrics: %s", err)
	}
	gardenContainers, err := gu.GetGardenContainers()
	if err != nil {
		return nil, fmt.Errorf("error listing garden containers: %v", err)
	}

	var cList = make([]*containers.Container, len(gardenContainers))
	handles := make([]string, len(gardenContainers))
	for i, gardenContainer := range gardenContainers {
		handles[i] = gardenContainer.Handle()
	}
	gardenContainerInfo, err := gu.cli.BulkInfo(handles)
	if err != nil {
		return nil, fmt.Errorf("error getting info for garden containers: %v", err)
	}
	gardenContainerMetrics, err := gu.cli.BulkMetrics(handles)
	if err != nil {
		return nil, fmt.Errorf("error getting metrics for garden containers: %v", err)
	}

	for i, handle := range handles {
		infoEntry := gardenContainerInfo[handle]
		if err := infoEntry.Err; err != nil {
			log.Debugf("could not get info for container %s: %v", handle, err)
			continue
		}
		metricsEntry := gardenContainerMetrics[handle]
		if err := metricsEntry.Err; err != nil {
			log.Debugf("could not get metrics for container %s: %v", handle, err)
			continue
		}
		container := containers.Container{
			Type:        "garden",
			ID:          handle,
			EntityID:    containers.BuildTaggerEntityName(handle),
			State:       infoEntry.Info.State,
			Excluded:    false,
			Created:     time.Now().Add(-metricsEntry.Metrics.Age).Unix(),
			AddressList: parseContainerPorts(infoEntry.Info),
		}
		cList[i] = &container
	}

	for _, container := range cList {
		if container.State != containers.ContainerActiveState {
			log.Debugf("Container %s not in state %s, skipping", container.ID[:12], containers.ContainerActiveState)
			continue
		}
		if !providers.ContainerImpl().ContainerExists(container.ID) {
			log.Debugf("Container %s not found, skipping", container.ID[:12])
			continue
		}
		limits, err := providers.ContainerImpl().GetContainerLimits(container.ID)
		if err != nil {
			log.Debugf("Cannot get limits for container %s: %s, skipping", container.ID[:12], err)
			continue
		}
		container.SetLimits(limits)
		gu.getContainerMetrics(container)
	}
	return cList, nil
}

// UpdateContainerMetrics updates the metric for a list of containers
func (gu *GardenUtil) UpdateContainerMetrics(cList []*containers.Container) error {
	if err := providers.ContainerImpl().Prefetch(); err != nil {
		return fmt.Errorf("could not fetch container metrics: %s", err)
	}
	for _, container := range cList {
		if container.State != containers.ContainerActiveState {
			log.Debugf("Container %s not in state %s, skipping", container.ID[:12], containers.ContainerActiveState)
			continue
		}
		if !providers.ContainerImpl().ContainerExists(container.ID) {
			log.Debugf("Container %s not found, skipping", container.ID[:12])
			continue
		}

		gu.getContainerMetrics(container)
	}
	return nil
}

// getContainerMetrics calls a ContainerImplementation, caller should always call Prefetch() before
func (gu *GardenUtil) getContainerMetrics(ctn *containers.Container) {
	metrics, err := providers.ContainerImpl().GetContainerMetrics(ctn.ID)
	if err != nil {
		log.Debugf("ContainerImplementation cannot get metrics for container %s, err: %s", ctn.ID[:12], err)
		return
	}
	ctn.SetMetrics(metrics)

	pids, err := providers.ContainerImpl().GetPIDs(ctn.ID)
	if err != nil {
		log.Debugf("ContainerImplementation cannot get PIDs for container %s, err: %s", ctn.ID[:12], err)
		return
	}
	ctn.Pids = pids

	networkMetrics, err := providers.ContainerImpl().GetNetworkMetrics(ctn.ID, map[string]string{})
	if err != nil {
		log.Debugf("Cannot get network stats for container %s: %s", ctn.ID, err)
		return
	}
	ctn.Network = networkMetrics
}

func parseContainerPorts(info garden.ContainerInfo) []containers.NetworkAddress {
	var addresses = make([]containers.NetworkAddress, len(info.MappedPorts))
	for i, port := range info.MappedPorts {
		addresses[i] = containers.NetworkAddress{
			IP:       net.ParseIP(info.ExternalIP),
			Port:     int(port.HostPort),
			Protocol: "tcp",
		}
	}
	return addresses
}

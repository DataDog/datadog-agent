// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudfoundry

import (
	"fmt"
	"sync"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/client"
	"code.cloudfoundry.org/garden/client/connection"
	"github.com/DataDog/datadog-agent/pkg/config"
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
	// AppIDTagKey tag key for container tags. We carry both app_guid and app_id; this is because
	// we added app_guid initially here, but then we added space_id and org_id that have just "_id"
	// to be consistent with https://github.com/DataDog/datadog-firehose-nozzle; therefore we now
	// also include "app_id" to have a consistent set of tags that end with "_id".
	AppIDTagKey = "app_id"
	// OrgIDTagKey tag key for container tags
	// NOTE: we use "org_*" instead of "organization_* to have the tags consistent with
	// tags attached by https://github.com/DataDog/datadog-firehose-nozzle
	OrgIDTagKey = "org_id"
	// OrgNameTagKey tag key for container tags
	OrgNameTagKey = "org_name"
	// SpaceIDTagKey tag key for container tags
	SpaceIDTagKey = "space_id"
	// SpaceNameTagKey tag key for container tags
	SpaceNameTagKey = "space_name"
	// SidecarPresentTagKey tag key for container tags
	SidecarPresentTagKey = "sidecar_present"
	// SidecarCountTagKey tag key for container tags
	SidecarCountTagKey = "sidecar_count"
	// SegmentNameTagKey tag key for container tags
	SegmentNameTagKey = "segment_name"
	// SegmentIDTagKey tag key for container tags
	SegmentIDTagKey = "segment_id"
)

var (
	globalGardenUtil     *GardenUtil
	globalGardenUtilLock sync.Mutex
)

// GardenUtilInterface describes a wrapper for collecting local garden containers
type GardenUtilInterface interface {
	ListContainers() ([]garden.Container, error)
	GetContainersInfo(handles []string) (map[string]garden.ContainerInfoEntry, error)
	GetContainersMetrics(handles []string) (map[string]garden.ContainerMetricsEntry, error)
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

// ListContainers returns the list of running containers from the local garden API
func (gu *GardenUtil) ListContainers() ([]garden.Container, error) {
	return gu.cli.Containers(nil)
}

// GetContainersInfo returns ContainerInfo per handle
func (gu *GardenUtil) GetContainersInfo(handles []string) (map[string]garden.ContainerInfoEntry, error) {
	gardenContainerInfo, err := gu.cli.BulkInfo(handles)
	if err != nil {
		return nil, fmt.Errorf("error getting info for garden containers: %v", err)
	}

	return gardenContainerInfo, nil
}

// GetContainersMetrics returns ContainerMetrics per handle
func (gu *GardenUtil) GetContainersMetrics(handles []string) (map[string]garden.ContainerMetricsEntry, error) {
	gardenContainerMetrics, err := gu.cli.BulkMetrics(handles)
	if err != nil {
		return nil, fmt.Errorf("error getting metrics for garden containers: %v", err)
	}

	return gardenContainerMetrics, nil
}

// ContainersToHandles returns a list of handles from a list of garden.Container
func ContainersToHandles(containers []garden.Container) []string {
	handles := make([]string, len(containers))
	for i, gardenContainer := range containers {
		handles[i] = gardenContainer.Handle()
	}
	return handles
}

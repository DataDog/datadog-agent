// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	trainingRunIDTag   = "training_run_id"
	trainingGroupIDTag = "training_group_id"
)

// TrainingInfoCache manages the training job identifiers of GPU processes received from system-probe.
type TrainingInfoCache struct {
	client     *sysprobeclient.CheckClient
	deviceTags map[string][]string
}

// NewTrainingInfoCache creates a training info cache using client.
func NewTrainingInfoCache(client *sysprobeclient.CheckClient) *TrainingInfoCache {
	return &TrainingInfoCache{client: client}
}

// Refresh fetches the training job identifiers from system-probe and caches them as tags per device.
func (c *TrainingInfoCache) Refresh() error {
	if c.client == nil {
		return errors.New("system-probe client is nil")
	}

	infos, err := sysprobeclient.GetEndpoint[[]model.TrainingInfo](c.client, "/training-info", sysconfig.GPUMonitoringModule)
	if err != nil {
		c.deviceTags = nil
		if sysprobeclient.IgnoreStartupError(err) == nil {
			log.Debugf("System-probe GPU training info endpoint not ready yet")
			return nil
		}
		return fmt.Errorf("get training info from system-probe: %w", err)
	}

	c.deviceTags = trainingInfoDeviceTags(infos)
	return nil
}

// DeviceTags returns the training job tags of the processes using the device, from the latest refresh.
func (c *TrainingInfoCache) DeviceTags(deviceUUID string) []string {
	return c.deviceTags[deviceUUID]
}

// trainingInfoDeviceTags builds the sorted, deduplicated training job tags of each device.
func trainingInfoDeviceTags(infos []model.TrainingInfo) map[string][]string {
	deviceTags := make(map[string][]string)
	for _, info := range infos {
		tags := deviceTags[info.DeviceUUID]
		if info.TrainingRunID != "" {
			tags = append(tags, trainingRunIDTag+":"+info.TrainingRunID)
		}
		if info.TrainingGroupID != "" {
			tags = append(tags, trainingGroupIDTag+":"+info.TrainingGroupID)
		}
		deviceTags[info.DeviceUUID] = tags
	}

	for deviceUUID, tags := range deviceTags {
		slices.Sort(tags)
		deviceTags[deviceUUID] = slices.Compact(tags)
	}
	return deviceTags
}

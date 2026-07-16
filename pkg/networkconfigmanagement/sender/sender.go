// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package sender provides a wrapper around the sender.Sender to send network device configuration data
package sender

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/benbjohnson/clock"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	ncmreport "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/report"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/utils"
)

const (
	ncmCheckDurationMetric             = "datadog.ncm.check_duration"
	ncmCheckIntervalMetric             = "datadog.ncm.check_interval"
	ncmCheckInventoryEntriesSentMetric = "datadog.ncm.inventory.entries_sent"
	ncmConfigSizeMetric                = "ncm.config_size"
	ncmStoreConfigsEvictedMetric       = "datadog.ncm.store.configs_evicted"

	ncmRunningConfigTypeTag = "config_type:running"
	ncmStartupConfigTypeTag = "config_type:startup"
)

// NCMSender is a wrapper around the sender.Sender to send network device configuration data
type NCMSender struct {
	Sender        sender.Sender
	namespace     string
	agentHostname string
	deviceTags    []string
	clock         clock.Clock
}

// NewNCMSender creates a new NCMSender
func NewNCMSender(sender sender.Sender, namespace string, clock clock.Clock, agentHostname string) *NCMSender {
	return &NCMSender{
		Sender:        sender,
		namespace:     namespace,
		clock:         clock,
		agentHostname: agentHostname,
	}
}

// SetDeviceTags sets the device tags for the sender to use for metrics submission
func (s *NCMSender) SetDeviceTags(deviceTags []string) {
	s.deviceTags = deviceTags
}

func (s *NCMSender) getDeviceTags() []string {
	return utils.CopyStrings(s.deviceTags)
}

// SendStoreEvictionMetrics sends metrics about a local config store eviction run.
// Eviction is a store-wide event, not scoped to a single device, so device tags are
// omitted; only common agent tags plus a status tag are attached.
func (s *NCMSender) SendStoreEvictionMetrics(evictedCount int, err error) {
	tags := utils.GetCommonAgentTags()
	if err != nil {
		tags = append(tags, "status:error")
	} else {
		tags = append(tags, "status:ok")
	}
	s.Sender.Count(ncmStoreConfigsEvictedMetric, float64(evictedCount), s.agentHostname, tags)
}

// SendNCMCheckMetrics sends metrics about the check itself to Datadog
func (s *NCMSender) SendNCMCheckMetrics(startTime time.Time, lastCheckTime time.Time, success bool) {
	tags := append(s.getDeviceTags(), utils.GetCommonAgentTags()...)
	if success {
		tags = append(tags, "status:ok")
	} else {
		tags = append(tags, "status:error")
	}
	duration := s.clock.Since(startTime).Seconds()
	s.Sender.Gauge(ncmCheckDurationMetric, duration, s.agentHostname, tags)

	if !lastCheckTime.IsZero() {
		interval := startTime.Sub(lastCheckTime).Seconds()
		s.Sender.Gauge(ncmCheckIntervalMetric, interval, s.agentHostname, tags)
	}
}

func (s *NCMSender) sendNCMPayloadMetrics(payload ncmreport.NCMPayload) {
	tags := utils.GetCommonAgentTags()
	s.Sender.Count(ncmCheckInventoryEntriesSentMetric, float64(len(payload.Inventories)), s.agentHostname, tags)
}

// SendMetricsFromExtractedMetadata sends metrics from data extracted from the device config after processing
func (s *NCMSender) SendMetricsFromExtractedMetadata(metadata profile.ExtractedMetadata, configType types.ConfigType) {
	tags := append(s.getDeviceTags(), utils.GetCommonAgentTags()...)
	switch configType {
	case types.RUNNING:
		tags = append(s.getDeviceTags(), ncmRunningConfigTypeTag)
	case types.STARTUP:
		tags = append(s.getDeviceTags(), ncmStartupConfigTypeTag)
	}
	// if config size was extracted, submit the metric
	if metadata.ConfigSize != 0 {
		s.Sender.Gauge(ncmConfigSizeMetric, float64(metadata.ConfigSize), s.agentHostname, tags)
	}
}

// SendNCMPayload sends the network device configuration payload to event platform
func (s *NCMSender) SendNCMPayload(payload ncmreport.NCMPayload) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.Sender.EventPlatformEvent(payloadBytes, eventplatform.EventTypeNetworkConfigManagement)
	// TODO: send metrics about the config retrieval?
	s.sendNCMPayloadMetrics(payload)
	return nil
}

// SendDeviceMetadata sends device metadata to NDM intake
func (s *NCMSender) SendDeviceMetadata(deviceID string, deviceIP string) error {
	payload := devicemetadata.NetworkDevicesMetadata{
		Namespace:   s.namespace,
		Integration: integrations.NetworkConfigManagement,
		Devices: []devicemetadata.DeviceMetadata{
			{
				ID:        deviceID,
				IPAddress: deviceIP,
				Status:    devicemetadata.DeviceStatusReachable,
			},
		},
		CollectTimestamp: s.clock.Now().Unix(),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshalling device metadata: %w", err)
	}
	s.Sender.EventPlatformEvent(payloadBytes, eventplatform.EventTypeNetworkDevicesMetadata)
	return nil
}

// Commit commits the sender (important to ensure data is flushed/sent
func (s *NCMSender) Commit() {
	s.Sender.Commit()
}

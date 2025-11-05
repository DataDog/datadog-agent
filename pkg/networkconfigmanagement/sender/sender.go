// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

// Package sender provides a wrapper around the sender.Sender to send network device configuration data
package sender

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	ncmreport "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/report"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/utils"
	"github.com/benbjohnson/clock"
)

const (
	ncmCheckDurationMetric  = "datadog.ncm.check_duration"
	ncmCheckIntervalMetric  = "datadog.ncm.check_interval"
	ncmConfigSizeMetric     = "ncm.config_size"
	ncmRunningConfigTypeTag = "config_type:running"
	ncmStartupConfigTypeTag = "config_type:startup"
)

// NCMSender is a wrapper around the sender.Sender to send network device configuration data
type NCMSender struct {
	Sender     sender.Sender
	namespace  string
	hostname   string // TODO: get the hostname to use
	deviceTags []string
	clock      clock.Clock
}

// NewNCMSender creates a new NCMSender
func NewNCMSender(sender sender.Sender, namespace string, clock clock.Clock) *NCMSender {
	return &NCMSender{
		Sender:    sender,
		namespace: namespace,
		clock:     clock,
	}
}

// SetDeviceTags sets the device tags for the sender to use for metrics submission
func (s *NCMSender) SetDeviceTags(deviceTags []string) {
	s.deviceTags = deviceTags
}

func (s *NCMSender) getDeviceTags() []string {
	return utils.CopyStrings(s.deviceTags)
}

// SendNCMCheckMetrics sends metrics about the check itself to Datadog
func (s *NCMSender) SendNCMCheckMetrics(startTime time.Time, lastCheckTime time.Time) {
	tags := append(s.getDeviceTags(), utils.GetCommonAgentTags()...)
	duration := s.clock.Since(startTime).Seconds()
	s.Sender.Gauge(ncmCheckDurationMetric, duration, s.hostname, tags)

	if !lastCheckTime.IsZero() {
		interval := startTime.Sub(lastCheckTime).Seconds()
		s.Sender.Gauge(ncmCheckIntervalMetric, interval, s.hostname, tags)
	}
}

// SendMetricsFromExtractedMetadata sends metrics from data extracted from the device config after processing
func (s *NCMSender) SendMetricsFromExtractedMetadata(metadata profile.ExtractedMetadata, configType ncmreport.ConfigType) {
	tags := append(s.getDeviceTags(), utils.GetCommonAgentTags()...)
	switch configType {
	case ncmreport.RUNNING:
		tags = append(s.getDeviceTags(), ncmRunningConfigTypeTag)
	case ncmreport.STARTUP:
		tags = append(s.getDeviceTags(), ncmStartupConfigTypeTag)
	}
	// if config size was extracted, submit the metric
	if metadata.ConfigSize != 0 {
		s.Sender.Gauge(ncmConfigSizeMetric, float64(metadata.ConfigSize), s.hostname, tags)
	}
}

// SendNCMConfig sends the network device configuration payload to event platform
func (s *NCMSender) SendNCMConfig(payload ncmreport.NCMPayload) error {
	payloadBytes, err := json.Marshal(payload)
	fmt.Println(string(payloadBytes))
	if err != nil {
		return err
	}
	s.Sender.EventPlatformEvent(payloadBytes, eventplatform.EventTypeNetworkConfigManagement)
	// TODO: send metrics about the config retrieval?
	return nil
}

// Commit commits the sender (important to ensure data is flushed/sent
func (s *NCMSender) Commit() {
	s.Sender.Commit()
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package sender provides a common interface for sending network device
// metrics and metadata to the Datadog Agent.
// It abstracts the underlying sender implementation, allowing for
// different sender types (e.g., for different network device integrations)
// to be used interchangeably.
package sender

import (
	"maps"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// timestampExpiration defines the expiration time for timestamps
// to ensure we continue to send metrics even if the timestamp is not updated
// for a long time. This is useful for devices that may not update their
// metrics frequently.
const timestampExpiration = 6 * time.Hour

// TimeNow useful for mocking
var TimeNow = time.Now

// Sender interface defines the methods for sending metrics and metadata
// for network devices. It extends the sender.Sender interface
// to include additional functionality specific to NDM.
type Sender interface {
	sender.Sender
	// GaugeWithTimestampWrapper wraps sender GaugeWithTimestamp with error handling
	GaugeWithTimestampWrapper(name string, value float64, tags []string, ts float64)
	// CountWithTimestamp wraps sender CountWithTimestamp with error handling
	CountWithTimestampWrapper(name string, value float64, tags []string, ts float64)
	// UpdateTimestamps updates the last sent timestamps
	UpdateTimestamps(newTimestamps map[string]float64)
	// SetDeviceTagsMap sets the device tags map
	SetDeviceTagsMap(deviceTags map[string][]string)
	// GetDeviceTags returns the device tags for a given IP address
	GetDeviceTags(defaultIPTag string, deviceIP string) []string
	// ShouldSendEntry checks if a metric entry should be sent based on its timestamp
	ShouldSendEntry(key string, ts float64) bool
}

// IntegrationSender implements the Sender interface for network device integrations
type IntegrationSender struct {
	sender.Sender
	integration  string
	namespace    string
	lastTimeSent map[string]float64
	deviceTags   map[string][]string
}

// NewSender returns a new IntegrationSender
func NewSender(sender sender.Sender, integration string, namespace string) *IntegrationSender {
	return &IntegrationSender{
		Sender:       sender,
		integration:  integration,
		namespace:    namespace,
		lastTimeSent: make(map[string]float64),
	}
}

// GaugeWithTimestampWrapper wraps sender GaugeWithTimestamp with error handling
func (s *IntegrationSender) GaugeWithTimestampWrapper(name string, value float64, tags []string, ts float64) {
	err := s.GaugeWithTimestamp(name, value, "", tags, ts)
	if err != nil {
		log.Warnf("Error sending %s metric %s : %s", s.integration, name, err)
	}
}

// CountWithTimestampWrapper wraps sender CountWithTimestamp with error handling
func (s *IntegrationSender) CountWithTimestampWrapper(name string, value float64, tags []string, ts float64) {
	err := s.CountWithTimestamp(name, value, "", tags, ts)
	if err != nil {
		log.Warnf("Error sending %s metric %s : %s", s.integration, name, err)
	}
}

// UpdateTimestamps updates the last sent timestamps
// This is used to avoid sending the same metric multiple times
// within a short period of time.
func (s *IntegrationSender) UpdateTimestamps(newTimestamps map[string]float64) {
	maps.Copy(s.lastTimeSent, newTimestamps)
}

// SetDeviceTagsMap sets the device tags map
func (s *IntegrationSender) SetDeviceTagsMap(deviceTags map[string][]string) {
	s.deviceTags = deviceTags
}

// GetDeviceTags returns the device tags for a given IP address
// If no tags are found, it returns a default tag with the IP address and the configured namespace.
func (s *IntegrationSender) GetDeviceTags(defaultIPTag string, deviceIP string) []string {
	tags, ok := s.deviceTags[deviceIP]
	if !ok {
		return []string{defaultIPTag + ":" + deviceIP, "device_namespace:" + s.namespace}
	}
	return utils.CopyStrings(tags)
}

// ShouldSendEntry checks if a metric entry should be sent based on its timestamp
// It compares the current timestamp with the last sent timestamp for the given key.
func (s *IntegrationSender) ShouldSendEntry(key string, ts float64) bool {
	lastTs, ok := s.lastTimeSent[key]
	if ok && lastTs >= ts {
		return false
	}
	return true
}

// Commit commits the current sender state and expires old timestamps
func (s *IntegrationSender) Commit() {
	s.Sender.Commit()
	s.expireTimeSent()
}

func (s *IntegrationSender) expireTimeSent() {
	expireTs := TimeNow().Add(-timestampExpiration).UTC().Unix()
	for key, ts := range s.lastTimeSent {
		if ts < float64(expireTs) {
			delete(s.lastTimeSent, key)
		}
	}
}

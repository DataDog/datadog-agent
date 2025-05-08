// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package report implements Versa metadata and metrics reporting
package report

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client"
	ndmsender "github.com/DataDog/datadog-agent/pkg/networkdevice/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultIPTag         = "device_ip"
	versaMetricPrefix    = "versa."
	versaTimestampFormat = "2006-01-02 15:04:05"
	timestampExpiration  = 6 * time.Hour
)

// Sender implements methods for sending Versa metrics and metadata
type Sender struct {
	ndmsender.Sender
	namespace string
}

// NewSender returns a new VersaSender
func NewSender(sender sender.Sender, namespace string) *Sender {
	return &Sender{
		Sender:    ndmsender.NewSender(sender, "versa", namespace),
		namespace: namespace,
	}
}

// SendDeviceMetrics sends device hardware metrics
func (s *Sender) SendDeviceMetrics(appliances []client.Appliance) {
	newTimestamps := make(map[string]float64)

	for _, appliance := range appliances {
		tags := s.GetDeviceTags(defaultIPTag, appliance.IPAddress)
		key := ndmsender.GetMetricKey("device_metrics", appliance.IPAddress)

		// Convert lastUpdatedTime to unix timestamp
		lastUpdatedTime, err := parseTimestamp(appliance.LastUpdatedTime)
		if err != nil {
			log.Warnf("Error parsing timestamp %s: %s. Sending device metrics...", appliance.LastUpdatedTime, err)
			lastUpdatedTime = float64(time.Now().UnixMilli())
		}
		if !s.ShouldSendEntry(key, lastUpdatedTime) {
			// If the timestamp is before the max timestamp already sent, do not re-send
			continue
		}
		ndmsender.SetNewSentTimestamp(newTimestamps, key, lastUpdatedTime)

		ts := lastUpdatedTime / 1000 // convert to seconds
		cpuLoad := appliance.Hardware.CPULoad

		// Parse memory metrics
		sendMemUsage := true
		memFree, err := parseSize(appliance.Hardware.FreeMemory)
		if err != nil {
			log.Warnf("Error parsing FreeMemory %s: %s", appliance.Hardware.FreeMemory, err)
			sendMemUsage = false
			memFree = 0
		}
		memTotal, err := parseSize(appliance.Hardware.Memory)
		if err != nil {
			log.Warnf("Error parsing Memory %s: %s", appliance.Hardware.Memory, err)
			sendMemUsage = false
			memTotal = 1
		}
		memUsage := 100 - (memFree / memTotal * float64(100))

		// Parse disk metrics
		sendDiskUsage := true
		diskFree, err := parseSize(appliance.Hardware.FreeDisk)
		if err != nil {
			log.Warnf("Error parsing FreeDisk %s: %s", appliance.Hardware.FreeDisk, err)
			sendDiskUsage = false
			diskFree = 0
		}
		diskSize, err := parseSize(appliance.Hardware.DiskSize)
		if err != nil {
			log.Warnf("Error parsing DiskSize %s: %s", appliance.Hardware.DiskSize, err)
			sendDiskUsage = false
			diskSize = 1
		}
		diskUsage := 100 - (diskFree / diskSize * float64(100))

		s.GaugeWithTimestampWrapper(versaMetricPrefix+"cpu.usage", float64(cpuLoad), tags, ts) // Using CPUUserNew and CPUSystem (not new...) to match vManage UI
		if sendMemUsage {
			s.GaugeWithTimestampWrapper(versaMetricPrefix+"memory.usage", memUsage, tags, ts)
		}
		if sendDiskUsage {
			s.GaugeWithTimestampWrapper(versaMetricPrefix+"disk.usage", diskUsage, tags, ts)
		}
	}

	s.UpdateTimestamps(newTimestamps)
}

// SendDeviceStatusMetrics sends device status metrics
func (s *Sender) SendDeviceStatusMetrics(deviceStatus map[string]float64) {
	for device, status := range deviceStatus {
		tags := s.GetDeviceTags(defaultIPTag, device)
		s.Gauge(versaMetricPrefix+"device.reachable", status, "", tags)
	}
}

// SendUptimeMetrics sends device uptime metrics
func (s *Sender) SendUptimeMetrics(uptimes map[string]float64) {
	for device, uptime := range uptimes {
		tags := s.GetDeviceTags(defaultIPTag, device)
		s.Gauge(versaMetricPrefix+"device.uptime", uptime, "", tags)
	}
}

// parseTimestamp parses a timestamp string in the format "2006-01-02 15:04:05.0" and returns the unix timestamp in milliseconds
// If the timestamp is invalid, it returns the current time in milliseconds
func parseTimestamp(timestamp string) (float64, error) {
	t, err := time.Parse(versaTimestampFormat, timestamp)
	if err != nil {
		return float64(time.Now().UnixMilli()), err
	}
	return float64(t.UnixMilli()), nil
}

// parseSize parses a size string and returns the size in bytes
// Supported formats are: "1KiB", "1MiB", "1GiB", "1Tib", "1Pib", "1Eib" and "1B"
func parseSize(sizeString string) (float64, error) {
	// bytes will be checked for separately
	units := map[string]float64{
		"kib": 1 << 10,
		"mib": 1 << 20,
		"gib": 1 << 30,
		"tib": 1 << 40,
		"pib": 1 << 50,
		"eib": 1 << 60,
	}

	normalizedSize := strings.TrimSpace(strings.ToLower(sizeString))
	var suffix string
	if len(normalizedSize) >= 3 {
		suffix = normalizedSize[len(normalizedSize)-3:]

		// check if suffix is in units map
		if factor, ok := units[suffix]; ok {
			size, err := strconv.ParseFloat(normalizedSize[:len(normalizedSize)-len(suffix)], 64)
			if err != nil {
				// If parsing fails, see if it's a number longer than 3 digits but has bytes
				return 0, fmt.Errorf("error parsing size %s: %w", sizeString, err)
			}
			return size * factor, nil
		}
	}

	// check if suffix is bytes
	if strings.HasSuffix(normalizedSize, "b") {
		size, err := strconv.ParseFloat(normalizedSize[:len(normalizedSize)-1], 64)
		if err != nil {
			return 0, fmt.Errorf("error parsing size %s: %w", sizeString, err)
		}
		return size, nil
	}

	return 0, fmt.Errorf("no matching units found for: %s", sizeString)
}

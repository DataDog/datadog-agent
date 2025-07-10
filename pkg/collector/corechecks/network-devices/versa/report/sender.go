// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package report implements Versa metadata and metrics reporting
package report

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client"
	ndmsender "github.com/DataDog/datadog-agent/pkg/networkdevice/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/multierr"
)

const (
	defaultIPTag                  = "device_ip"
	versaMetricPrefix             = "versa."
	versaTimestampFormat          = "2006-01-02 15:04:05.0"
	alternateVersaTimestampFormat = "2006/01/02 15:04:05"
	timestampExpiration           = 6 * time.Hour
)

var (
	versaUptimeRegex = regexp.MustCompile(`(?i)(\S+)\s+(year?|days?|hours?|minutes?|seconds?)`)
)

type (
	// Sender implements methods for sending Versa metrics and metadata
	Sender struct {
		ndmsender.Sender
		namespace string
	}

	partition struct {
		Name      string
		Size      float64
		Free      float64
		UsedRatio float64
	}
)

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
			lastUpdatedTime = float64(TimeNow().UnixMilli())
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
		if status > 0 {
			s.Gauge(versaMetricPrefix+"device.unreachable", 0, "", tags)
		} else {
			s.Gauge(versaMetricPrefix+"device.unreachable", 1, "", tags)
		}
	}
}

// SendUptimeMetrics sends device uptime metrics
func (s *Sender) SendUptimeMetrics(uptimes map[string]float64) {
	for device, uptime := range uptimes {
		tags := s.GetDeviceTags(defaultIPTag, device)
		s.Gauge(versaMetricPrefix+"device.uptime", uptime, "", tags)
	}
}

// SendSLAMetrics sends SLA metrics retrieved from Versa Analytics
func (s *Sender) SendSLAMetrics(slaMetrics []client.SLAMetrics, deviceNameToIDMap map[string]string) {
	for _, slaMetricsResponse := range slaMetrics {
		var tags = []string{
			fmt.Sprintf("local_site:%s", slaMetricsResponse.LocalSite),
			fmt.Sprintf("remote_site:%s", slaMetricsResponse.RemoteSite),
			fmt.Sprintf("local_access_circuit:%s", slaMetricsResponse.LocalAccessCircuit),
			fmt.Sprintf("remote_access_circuit:%s", slaMetricsResponse.RemoteAccessCircuit),
			fmt.Sprintf("forwarding_class:%s", slaMetricsResponse.ForwardingClass),
		}
		if deviceIP, ok := deviceNameToIDMap[slaMetricsResponse.LocalSite]; ok {
			tags = append(tags, s.GetDeviceTags(defaultIPTag, deviceIP)...)
		}
		s.Gauge(versaMetricPrefix+"sla.delay", slaMetricsResponse.Delay, "", tags)
		s.Gauge(versaMetricPrefix+"sla.fwd_delay_var", slaMetricsResponse.FwdDelayVar, "", tags)
		s.Gauge(versaMetricPrefix+"sla.rev_delay_var", slaMetricsResponse.RevDelayVar, "", tags)
		s.Gauge(versaMetricPrefix+"sla.fwd_loss_ratio", slaMetricsResponse.FwdLossRatio, "", tags)
		s.Gauge(versaMetricPrefix+"sla.rev_loss_ratio", slaMetricsResponse.RevLossRatio, "", tags)
		s.Gauge(versaMetricPrefix+"sla.pdu_loss_ratio", slaMetricsResponse.PDULossRatio, "", tags)
	}
}

// SendDirectorUptimeMetrics sends director uptime metrics
func (s *Sender) SendDirectorUptimeMetrics(director *client.DirectorStatus) {
	ipAddress, err := director.IPAddress()
	if err != nil {
		log.Errorf("Error getting director IP address: %s", err)
		return
	}
	tags := s.GetDeviceTags(defaultIPTag, ipAddress)

	appUptime, err := parseUptimeString(director.SystemUpTime.ApplicationUpTime)
	if err != nil {
		log.Errorf("error parsing director application uptime: %v", err)
	} else {
		s.Gauge(versaMetricPrefix+"device.uptime", appUptime, "", append(tags, "type:application"))
	}

	sysUptime, err := parseUptimeString(director.SystemUpTime.SysProcUptime)
	if err != nil {
		log.Errorf("error parsing director system uptime: %v", err)
	} else {
		s.Gauge(versaMetricPrefix+"device.uptime", sysUptime, "", append(tags, "type:sys_proc"))
	}
}

// SendDirectorStatus sends the director status metric
func (s *Sender) SendDirectorStatus(director *client.DirectorStatus) {
	ipAddress, err := director.IPAddress()
	if err != nil {
		log.Errorf("Error getting director IP address: %s", err)
		return
	}
	tags := s.GetDeviceTags(defaultIPTag, ipAddress)

	s.Gauge(versaMetricPrefix+"device.reachable", 1, "", tags)
	s.Gauge(versaMetricPrefix+"device.unreachable", 0, "", tags)
}

// SendDirectorDeviceMetrics sends director device metrics like CPU, memory, and disk usage
func (s *Sender) SendDirectorDeviceMetrics(director *client.DirectorStatus) {
	ipAddress, err := director.IPAddress()
	if err != nil {
		log.Errorf("Error getting director IP address, skipping device metrics: %s", err)
		return
	}
	tags := s.GetDeviceTags(defaultIPTag, ipAddress)

	// Convert lastUpdatedTime to unix timestamp
	lastUpdatedTime := float64(TimeNow().UnixMilli())

	ts := lastUpdatedTime / 1000 // convert to seconds
	cpuLoadString := director.SystemDetails.CPULoad
	cpuLoad, err := strconv.ParseFloat(cpuLoadString, 64)
	if err != nil {
		log.Errorf("Error parsing CPULoad %q for director %q, skipping versa.cpu.usage: %s", cpuLoadString, ipAddress, err)
	} else {
		s.GaugeWithTimestampWrapper(versaMetricPrefix+"cpu.usage", cpuLoad, tags, ts)
	}

	// Parse memory metrics
	sendMemory := true
	memFree, err := parseSize(director.SystemDetails.MemoryFree)
	if err != nil {
		log.Errorf("Error parsing FreeMemory %q for director %q, skipping versa.memory.usage: %s", director.SystemDetails.MemoryFree, ipAddress, err)
		sendMemory = false
	}
	memTotal, err := parseSize(director.SystemDetails.Memory)
	if err != nil {
		log.Errorf("Error parsing Memory %q for director %q, skipping versa.memory.usage: %s", director.SystemDetails.Memory, ipAddress, err)
		sendMemory = false
	}
	if sendMemory {
		memUsage := 100 - (memFree / memTotal * float64(100))
		s.GaugeWithTimestampWrapper(versaMetricPrefix+"memory.usage", memUsage, tags, ts)
	}

	// Parse disk metrics
	diskPartitions, err := parseDiskUsage(director.SystemDetails.DiskUsage)
	if err != nil {
		log.Errorf("Error parsing DiskUsage %q for director %q: %s", director.SystemDetails.DiskUsage, ipAddress, err)
	}
	// loop over diskPartitions no matter what,
	// we may have been able to parse some metrics
	for _, partition := range diskPartitions {
		s.GaugeWithTimestampWrapper(versaMetricPrefix+"disk.usage", partition.UsedRatio, append(tags, "partition:"+partition.Name), ts)
	}
}

// parseTimestamp parses a timestamp string in the Versa formats and returns the unix timestamp in milliseconds
// If the timestamp is invalid, it returns the current time in milliseconds
func parseTimestamp(timestamp string) (float64, error) {
	t, err := time.Parse(versaTimestampFormat, timestamp)
	if err != nil {
		// If parsing fails, try the alternate format
		t, err = time.Parse(alternateVersaTimestampFormat, timestamp)
		if err != nil {
			return float64(TimeNow().UnixMilli()), fmt.Errorf("error parsing timestamp: %w", err)
		}
	}
	return float64(t.UnixMilli()), nil
}

// parseSize parses a size string and returns the size in bytes
// Supported formats are: "1KiB", "1MiB", "1GiB", "1Tib", "1Pib", "1Eib" and "1B"
func parseSize(sizeString string) (float64, error) {
	// bytes will be checked for separately
	units := map[string]float64{
		"b":   1,
		"kib": 1 << 10,
		"mib": 1 << 20,
		"gib": 1 << 30,
		"tib": 1 << 40,
		"pib": 1 << 50,
		"eib": 1 << 60,
		"kb":  1e3,
		"mb":  1e6,
		"gb":  1e9,
		"tb":  1e12,
		"pb":  1e15,
		"eb":  1e18,
	}

	normalizedSize := strings.TrimSpace(strings.ToLower(sizeString))
	longestUnit := ""
	largestFactor := 1.0
	for unit, factor := range units {
		if strings.HasSuffix(normalizedSize, unit) {
			if len(unit) > len(longestUnit) {
				longestUnit = unit
				largestFactor = factor
			}
		}
	}
	// Remove the unit from the size string
	size, err := strconv.ParseFloat(normalizedSize[:len(normalizedSize)-len(longestUnit)], 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing size %q: %w", sizeString, err)
	}
	return size * largestFactor, nil
}

func parseUptimeString(uptime string) (float64, error) {
	uptime = strings.TrimSuffix(uptime, ".") // Remove the trailing period
	matches := versaUptimeRegex.FindAllStringSubmatch(uptime, -1)

	if len(matches) == 0 {
		return 0, fmt.Errorf("no valid time components found in uptime string: %s", uptime)
	}

	var total time.Duration
	for _, match := range matches {
		if len(match) < 3 {
			return 0, fmt.Errorf("invalid uptime format: %s", uptime)
		}
		valueStr := match[1]
		unit := strings.ToLower(match[2])

		// Check if the value string contains any non-digit characters
		for _, char := range valueStr {
			if !strings.ContainsRune("0123456789", char) {
				return 0, fmt.Errorf("invalid numeric value %q in uptime component: contains non-digit character", valueStr)
			}
		}

		value, err := strconv.Atoi(valueStr)
		if err != nil {
			return 0, fmt.Errorf("invalid number %s: %v", valueStr, err)
		}

		switch unit {
		// not clear if we will ever get years from the API, but if we do
		// use 365 days as an estimate.
		case "year", "years":
			total += time.Duration(value) * 365 * 24 * time.Hour
		case "day", "days":
			total += time.Duration(value) * 24 * time.Hour
		case "hour", "hours":
			total += time.Duration(value) * time.Hour
		case "minute", "minutes":
			total += time.Duration(value) * time.Minute
		case "second", "seconds":
			total += time.Duration(value) * time.Second
		}
	}

	return math.Round(float64(total) / float64(time.Millisecond) / 10), nil // In hundredths of a second, to match SNMP
}

// parseDiskUsage takes a Versa director disk usage string and parses it
// per parition.
// e.g. partition=root,size=50GB,free=10GB,usedRatio=20.0;partition=opt,size=30GB,free=15GB,usedRatio=50.0
func parseDiskUsage(diskUsage string) ([]partition, error) {
	var partitions []partition
	entries := strings.Split(diskUsage, ";")
	if diskUsage == "" || len(entries) == 0 {
		return nil, fmt.Errorf("failed to parse diskUsage string: %s", diskUsage)
	}

	var partialParseErrs error
	partialParseErrCount := 0
	for _, entry := range entries {
		fields := strings.Split(entry, ",")
		p := partition{}
		for _, field := range fields {
			kv := strings.SplitN(field, "=", 2)
			if len(kv) != 2 {
				partialParseErrs = multierr.Append(partialParseErrs, fmt.Errorf("failed to parse diskUsage partition: %s", field))
				partialParseErrCount++
				continue
			}
			key, value := kv[0], kv[1]
			switch key {
			case "partition":
				if value == "" {
					partialParseErrs = multierr.Append(partialParseErrs, fmt.Errorf("failed to parse parition name: %q", value))
					partialParseErrCount++
					continue
				}
				p.Name = value
			case "size":
				size, err := parseSize(value)
				if err != nil {
					partialParseErrs = multierr.Append(partialParseErrs, fmt.Errorf("failed to parse disk size: %s", value))
					partialParseErrCount++
					continue
				}
				p.Size = size
			case "free":
				free, err := parseSize(value)
				if err != nil {
					partialParseErrs = multierr.Append(partialParseErrs, fmt.Errorf("failed to parse disk free: %s", value))
					partialParseErrCount++
					continue
				}
				p.Free = free
			case "usedRatio":
				usedRatio, err := strconv.ParseFloat(value, 64)
				if err != nil {
					partialParseErrs = multierr.Append(partialParseErrs, fmt.Errorf("failed to parse disk used ratio: %s", value))
					partialParseErrCount++
					continue
				}
				p.UsedRatio = usedRatio
			}
		}
		partitions = append(partitions, p)
	}
	if partialParseErrCount == len(entries) {
		return nil, fmt.Errorf("no valid partitions found in diskUsage string: %s", diskUsage)
	}

	return partitions, partialParseErrs
}

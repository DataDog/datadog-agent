// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package report

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client"
	"github.com/stretchr/testify/mock"
)

// mockTimeNow mocks time.Now
var mockTimeNow = func() time.Time {
	layout := "2006-01-02 15:04:05"
	str := "2000-01-01 00:00:00"
	t, _ := time.Parse(layout, str)
	return t
}

// expectedMetric represents a metric that should be sent by the sender
type expectedMetric struct {
	name  string
	value float64
	tags  []string
	ts    float64
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
		errMsg   string
	}{
		{"2000-01-01 00:00:00.0", 946684800000, ""},
		{"2000/01/01 00:00:00", 946684800000, ""},
		{"invalid timestamp", 0, "error parsing timestamp"},
	}

	for _, test := range tests {
		result, err := parseTimestamp(test.input)
		if err != nil {
			if test.errMsg == "" {
				t.Errorf("Unexpected error parsing timestamp %s: %v", test.input, err)
			} else if !strings.Contains(err.Error(), test.errMsg) {
				t.Errorf("Expected error message to contain %q, got %q", test.errMsg, err.Error())
			}
			continue
		}
		if result != test.expected {
			t.Errorf("Expected %2f, got %2f for input %s", test.expected, result, test.input)
		}
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
		errMsg   string
	}{
		{"1B", 1, ""},
		{"10B", 10, ""},
		{"999.8B", 999.8, ""},
		{"50.12KiB", 50.12 * 1024, ""},
		{"100MiB", 100 * 1024 * 1024, ""},
		{"80.09GiB", 80.09 * float64(1<<30), ""},
		{"", 0, "error parsing size"},
		{"1.5ZKiB", 0, "error parsing size"},
		{"9ZB", 0, "error parsing size"},
		{"120.05KB", 120.05e3, ""},
		{"101.5GB", 101.5e9, ""},
		{"1.5TB", 1.5e12, ""},
		{"1.5PB", 1.5e15, ""},
	}

	for _, test := range tests {
		result, err := parseSize(test.input)
		if err != nil {
			if test.errMsg == "" {
				t.Errorf("Unexpected error parsing size %s: %v", test.input, err)
			} else if !strings.Contains(err.Error(), test.errMsg) {
				t.Errorf("Expected error message to contain %q, got %q", test.errMsg, err.Error())
			}
			continue
		}
		if result != test.expected {
			t.Errorf("Expected %2f, got %2f for input %s", test.expected, result, test.input)
		}
	}
}

func TestParseUptimeString(t *testing.T) {
	tests := []struct {
		description     string
		input           string
		expectedYears   int64
		expectedDays    int64
		expectedHours   int64
		expectedMinutes int64
		expectedSeconds int64
		errMsg          string
	}{
		{
			description:     "Valid uptime, upper case",
			input:           "40 Years 299 Days 7 Hours 5 Minutes and 39 Seconds.",
			expectedYears:   40,
			expectedDays:    299,
			expectedHours:   7,
			expectedMinutes: 5,
			expectedSeconds: 39,
		},
		{
			description:     "Valid uptime",
			input:           "10 days 2 hours 3 minutes 4 seconds.",
			expectedDays:    10,
			expectedHours:   2,
			expectedMinutes: 3,
			expectedSeconds: 4,
		},
		{
			description:     "Valid uptime with years",
			input:           "5 years 278 days 16 hours 0 minutes 30 seconds.",
			expectedYears:   5,
			expectedDays:    278,
			expectedHours:   16,
			expectedMinutes: 0,
			expectedSeconds: 30,
		},
		{
			// TODO: do I like this? it's more flexible, but the
			// data coming from the API is likely wrong
			description:  "Uptime missing years, valid days",
			input:        " years 5 days",
			expectedDays: 5,
		},
		{
			description: "Invalid uptime, mixed letters/numbers",
			input:       "5x years",
			errMsg:      "invalid numeric value",
		},
		{
			description: "Invalid uptime, spelled out",
			input:       "five years",
			errMsg:      "invalid numeric value",
		},
		{
			description: "Invalid uptime, special characters",
			input:       "5! years",
			errMsg:      "invalid numeric value",
		},
		{
			description: "Invalid uptime, expect integers",
			input:       "5.5 years",
			errMsg:      "invalid numeric value",
		},
		{
			description: "Invalid uptime, negative number",
			input:       "-5 years",
			errMsg:      "invalid numeric value",
		},
		{
			description: "Invalid uptime, empty value",
			input:       " years",
			errMsg:      "no valid time components found",
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			result, err := parseUptimeString(test.input)
			if err != nil {
				if test.errMsg == "" {
					t.Errorf("Unexpected error parsing uptime %s: %v", test.input, err)
				} else if !strings.Contains(err.Error(), test.errMsg) {
					t.Errorf("Expected error message to contain %q, got %q", test.errMsg, err.Error())
				}
				return
			}

			if test.errMsg != "" {
				t.Errorf("Expected error containing %q but got none", test.errMsg)
				return
			}

			result *= float64(time.Millisecond) * 10
			years, days, hours, minutes, seconds := extractDurationComponents(time.Duration(result))

			if years != test.expectedYears ||
				days != test.expectedDays ||
				hours != test.expectedHours ||
				minutes != test.expectedMinutes ||
				seconds != test.expectedSeconds {
				t.Errorf("Result mismatch for %q:\nExpected: %d years %d days %d hours %d minutes %d seconds\nGot: %d years %d days %d hours %d minutes %d seconds",
					test.input,
					test.expectedYears, test.expectedDays, test.expectedHours, test.expectedMinutes, test.expectedSeconds,
					years, days, hours, minutes, seconds)
			}
		})
	}
}

// extractDurationComponents converts a duration into its constituent parts.
func extractDurationComponents(d time.Duration) (years, days, hours, minutes, seconds int64) {
	years = int64(d / (365 * 24 * time.Hour))
	d -= time.Duration(years) * 365 * 24 * time.Hour
	days = int64(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	hours = int64(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	minutes = int64(d / time.Minute)
	d -= time.Duration(minutes) * time.Minute
	seconds = int64(d / time.Second)

	return years, days, hours, minutes, seconds
}

func TestSendDeviceMetrics(t *testing.T) {
	TimeNow = mockTimeNow
	tests := []struct {
		name            string
		appliances      []client.Appliance
		expectedMetrics []expectedMetric
	}{
		{
			name: "Single appliance with valid metrics",
			appliances: []client.Appliance{
				{
					Name:            "test-appliance",
					IPAddress:       "192.168.1.1",
					LastUpdatedTime: "2024-01-01 00:00:00.0", // 1704067200
					Hardware: client.Hardware{
						CPULoad:    50,
						Memory:     "16GiB",
						FreeMemory: "8GiB",
						DiskSize:   "100GiB",
						FreeDisk:   "50GiB",
					},
				},
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "cpu.usage",
					value: 50,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    1704067200, // 2024-01-01 00:00:00.0
				},
				{
					name:  versaMetricPrefix + "memory.usage",
					value: 50, // (16GiB - 8GiB) / 16GiB * 100
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    1704067200,
				},
				{
					name:  versaMetricPrefix + "disk.usage",
					value: 50, // (100GiB - 50GiB) / 100GiB * 100
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    1704067200,
				},
			},
		},
		{
			name: "Single appliance with valid metrics, invalid timestamp",
			appliances: []client.Appliance{
				{
					Name:            "test-appliance",
					IPAddress:       "192.168.1.1",
					LastUpdatedTime: "2024INVALID 00:00:00.0", // Invalid, should fall back on TimeNow (946684800)
					Hardware: client.Hardware{
						CPULoad:    50,
						Memory:     "16GiB",
						FreeMemory: "8GiB",
						DiskSize:   "100GiB",
						FreeDisk:   "50GiB",
					},
				},
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "cpu.usage",
					value: 50,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    946684800, // 2000-01-01 00:00:00
				},
				{
					name:  versaMetricPrefix + "memory.usage",
					value: 50, // (16GiB - 8GiB) / 16GiB * 100
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    946684800,
				},
				{
					name:  versaMetricPrefix + "disk.usage",
					value: 50, // (100GiB - 50GiB) / 100GiB * 100
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    946684800,
				},
			},
		},
		{
			name: "Multiple appliances with valid metrics",
			appliances: []client.Appliance{
				{
					Name:            "appliance-1",
					IPAddress:       "192.168.1.1",
					LastUpdatedTime: "2024-01-01 00:00:00.0",
					Hardware: client.Hardware{
						CPULoad:    30,
						Memory:     "8GiB",
						FreeMemory: "4GiB",
						DiskSize:   "200GiB",
						FreeDisk:   "100GiB",
					},
				},
				{
					Name:            "appliance-2",
					IPAddress:       "192.168.1.2",
					LastUpdatedTime: "2024-01-01 00:00:00.0",
					Hardware: client.Hardware{
						CPULoad:    70,
						Memory:     "32GiB",
						FreeMemory: "16GiB",
						DiskSize:   "500GiB",
						FreeDisk:   "250GiB",
					},
				},
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "cpu.usage",
					value: 30,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    1704067200,
				},
				{
					name:  versaMetricPrefix + "memory.usage",
					value: 50, // (8GiB - 4GiB) / 8GiB * 100
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    1704067200,
				},
				{
					name:  versaMetricPrefix + "disk.usage",
					value: 50, // (200GiB - 100GiB) / 200GiB * 100
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    1704067200,
				},
				{
					name:  versaMetricPrefix + "cpu.usage",
					value: 70,
					tags:  []string{"device_ip:192.168.1.2", "device_namespace:default"},
					ts:    1704067200,
				},
				{
					name:  versaMetricPrefix + "memory.usage",
					value: 50, // (32GiB - 16GiB) / 32GiB * 100
					tags:  []string{"device_ip:192.168.1.2", "device_namespace:default"},
					ts:    1704067200,
				},
				{
					name:  versaMetricPrefix + "disk.usage",
					value: 50, // (500GiB - 250GiB) / 500GiB * 100
					tags:  []string{"device_ip:192.168.1.2", "device_namespace:default"},
					ts:    1704067200,
				},
			},
		},
		{
			name: "Appliance with invalid total memory",
			appliances: []client.Appliance{
				{
					Name:            "test-appliance",
					IPAddress:       "192.168.1.1",
					LastUpdatedTime: "2024-01-01 00:00:00.0",
					Hardware: client.Hardware{
						CPULoad:    50,
						Memory:     "invalid",
						FreeMemory: "8GiB",
						DiskSize:   "100GiB",
						FreeDisk:   "50GiB",
					},
				},
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "cpu.usage",
					value: 50,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    1704067200,
				},
				{
					name:  versaMetricPrefix + "disk.usage",
					value: 50, // (100GiB - 50GiB) / 100GiB * 100
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    1704067200,
				},
			},
		},
		{
			name: "Appliance with invalid free memory",
			appliances: []client.Appliance{
				{
					Name:            "test-appliance",
					IPAddress:       "192.168.1.1",
					LastUpdatedTime: "2024-01-01 00:00:00.0",
					Hardware: client.Hardware{
						CPULoad:    50,
						Memory:     "8GiB",
						FreeMemory: "INVALID",
						DiskSize:   "100GiB",
						FreeDisk:   "50GiB",
					},
				},
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "cpu.usage",
					value: 50,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    1704067200,
				},
				{
					name:  versaMetricPrefix + "disk.usage",
					value: 50, // (100GiB - 50GiB) / 100GiB * 100
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    1704067200,
				},
			},
		},
		{
			name: "Appliance with invalid disk size",
			appliances: []client.Appliance{
				{
					Name:            "test-appliance",
					IPAddress:       "192.168.1.1",
					LastUpdatedTime: "2024-01-01 00:00:00.0",
					Hardware: client.Hardware{
						CPULoad:    50,
						Memory:     "16GiB",
						FreeMemory: "8GiB",
						DiskSize:   "invalid",
						FreeDisk:   "50GiB",
					},
				},
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "cpu.usage",
					value: 50,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    1704067200,
				},
				{
					name:  versaMetricPrefix + "memory.usage",
					value: 50, // (16GiB - 8GiB) / 16GiB * 100
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    1704067200,
				},
			},
		},
		{
			name: "Appliance with invalid disk free",
			appliances: []client.Appliance{
				{
					Name:            "test-appliance",
					IPAddress:       "192.168.1.1",
					LastUpdatedTime: "2024-01-01 00:00:00.0",
					Hardware: client.Hardware{
						CPULoad:    50,
						Memory:     "16GiB",
						FreeMemory: "8GiB",
						DiskSize:   "130GiB",
						FreeDisk:   "INVALID",
					},
				},
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "cpu.usage",
					value: 50,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    1704067200,
				},
				{
					name:  versaMetricPrefix + "memory.usage",
					value: 50, // (16GiB - 8GiB) / 16GiB * 100
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    1704067200,
				},
			},
		},
		{
			name:            "Empty appliance list",
			appliances:      []client.Appliance{},
			expectedMetrics: []expectedMetric{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender("testID")
			mockSender.On("GaugeWithTimestamp", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			s := NewSender(mockSender, "default")
			s.SendDeviceMetrics(tt.appliances)

			for _, metric := range tt.expectedMetrics {
				mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", metric.name, metric.value, "", metric.tags, metric.ts)
			}

			// Verify no unexpected metrics were sent
			mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", len(tt.expectedMetrics))
		})
	}
}

func TestSendDirectorDeviceMetrics(t *testing.T) {
	TimeNow = mockTimeNow
	tests := []struct {
		name            string
		director        *client.DirectorStatus
		expectedMetrics []expectedMetric
	}{
		{
			name: "Director with valid metrics",
			director: &client.DirectorStatus{
				SystemDetails: client.DirectorSystemDetails{
					CPULoad:    "50.5",
					Memory:     "16GiB",
					MemoryFree: "8GiB",
					DiskUsage:  "partition=root,size=50GB,free=10GB,usedRatio=20.0;partition=opt,size=30GB,free=15GB,usedRatio=50.0",
				},
				HAConfig: client.DirectorHAConfig{
					MyVnfManagementIPs: []string{
						"192.168.1.1",
					},
				},
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "cpu.usage",
					value: 50.5,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    946684800, // 2000-01-01 00:00:00
				},
				{
					name:  versaMetricPrefix + "memory.usage",
					value: 50, // (16GiB - 8GiB) / 16GiB * 100
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    946684800,
				},
				{
					name:  versaMetricPrefix + "disk.usage",
					value: 20,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default", "partition:root"},
					ts:    946684800,
				},
				{
					name:  versaMetricPrefix + "disk.usage",
					value: 50,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default", "partition:opt"},
					ts:    946684800,
				},
			},
		},
		{
			name: "Director with invalid CPU load",
			director: &client.DirectorStatus{
				SystemDetails: client.DirectorSystemDetails{
					CPULoad:    "invalid",
					Memory:     "16GiB",
					MemoryFree: "8GiB",
					DiskUsage:  "partition=root,size=50GB,free=10GB,usedRatio=20.0;partition=opt,size=30GB,free=15GB,usedRatio=50.0",
				},
				HAConfig: client.DirectorHAConfig{
					MyVnfManagementIPs: []string{
						"192.168.1.1",
					},
				},
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "memory.usage",
					value: 50,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    946684800,
				},
				{
					name:  versaMetricPrefix + "disk.usage",
					value: 20,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default", "partition:root"},
					ts:    946684800,
				},
				{
					name:  versaMetricPrefix + "disk.usage",
					value: 50,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default", "partition:opt"},
					ts:    946684800,
				},
			},
		},
		{
			name: "Director with invalid memory metrics",
			director: &client.DirectorStatus{
				SystemDetails: client.DirectorSystemDetails{
					CPULoad:    "50.5",
					Memory:     "invalid",
					MemoryFree: "8GiB",
					DiskUsage:  "partition=root,size=50GB,free=10GB,usedRatio=20.0;partition=opt,size=30GB,free=15GB,usedRatio=50.0",
				},
				HAConfig: client.DirectorHAConfig{
					MyVnfManagementIPs: []string{
						"192.168.1.1",
					},
				},
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "cpu.usage",
					value: 50.5,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    946684800,
				},
				{
					name:  versaMetricPrefix + "disk.usage",
					value: 20,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default", "partition:root"},
					ts:    946684800,
				},
				{
					name:  versaMetricPrefix + "disk.usage",
					value: 50,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default", "partition:opt"},
					ts:    946684800,
				},
			},
		},
		{
			name: "Director with invalid disk usage",
			director: &client.DirectorStatus{
				SystemDetails: client.DirectorSystemDetails{
					CPULoad:    "50.5",
					Memory:     "16GiB",
					MemoryFree: "8GiB",
					DiskUsage:  "invalid",
				},
				HAConfig: client.DirectorHAConfig{
					MyVnfManagementIPs: []string{
						"192.168.1.1",
					},
				},
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "cpu.usage",
					value: 50.5,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    946684800,
				},
				{
					name:  versaMetricPrefix + "memory.usage",
					value: 50,
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default"},
					ts:    946684800,
				},
			},
		},
		{
			name: "Director with invalid IP address",
			director: &client.DirectorStatus{
				SystemDetails: client.DirectorSystemDetails{
					CPULoad:    "50.5",
					Memory:     "16GiB",
					MemoryFree: "8GiB",
					DiskUsage:  "partition=root,size=50GB,free=10GB,usedRatio=20.0;partition=opt,size=30GB,free=15GB,usedRatio=50.0",
				},
			},
			expectedMetrics: []expectedMetric{}, // No metrics should be sent if IP address is invalid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender("testID")
			mockSender.On("GaugeWithTimestamp", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			s := NewSender(mockSender, "default")
			s.SendDirectorDeviceMetrics(tt.director)

			for _, metric := range tt.expectedMetrics {
				mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", metric.name, metric.value, "", metric.tags, metric.ts)
			}

			// Verify no unexpected metrics were sent
			mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", len(tt.expectedMetrics))
		})
	}
}

func TestSendDirectorUptimeMetrics(t *testing.T) {
	tests := []struct {
		name            string
		director        *client.DirectorStatus
		expectedMetrics []expectedMetric
	}{
		{
			name: "Director with valid uptime metrics",
			director: &client.DirectorStatus{
				SystemUpTime: client.DirectorSystemUpTime{
					ApplicationUpTime: "1278 days 16 hours 0 minutes 30 seconds",
					SysProcUptime:     "10 days 2 hours 3 minutes 4 seconds",
				},
				HAConfig: client.DirectorHAConfig{
					MyVnfManagementIPs: []string{
						"192.168.1.1",
					},
				},
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "device.uptime",
					value: float64((1278*24*60*60 + 16*60*60 + 30) * 100), // Convert to hundredths of a second
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default", "type:application"},
				},
				{
					name:  versaMetricPrefix + "device.uptime",
					value: float64((10*24*60*60 + 2*60*60 + 3*60 + 4) * 100), // Convert to hundredths of a second
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default", "type:sys_proc"},
				},
			},
		},
		{
			name: "Director with invalid application uptime",
			director: &client.DirectorStatus{
				SystemUpTime: client.DirectorSystemUpTime{
					ApplicationUpTime: "invalid uptime",
					SysProcUptime:     "10 days 2 hours 3 minutes 4 seconds",
				},
				HAConfig: client.DirectorHAConfig{
					MyVnfManagementIPs: []string{
						"192.168.1.1",
					},
				},
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "device.uptime",
					value: float64((10*24*60*60 + 2*60*60 + 3*60 + 4) * 100), // Convert to hundredths of a second
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default", "type:sys_proc"},
				},
			},
		},
		{
			name: "Director with invalid system uptime",
			director: &client.DirectorStatus{
				SystemUpTime: client.DirectorSystemUpTime{
					ApplicationUpTime: "4278 days 16 hours 0 minutes 30 seconds",
					SysProcUptime:     "invalid uptime",
				},
				HAConfig: client.DirectorHAConfig{
					MyVnfManagementIPs: []string{
						"192.168.1.1",
					},
				},
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "device.uptime",
					value: float64((4278*24*60*60 + 16*60*60 + 30) * 100), // Convert to hundredths of a second
					tags:  []string{"device_ip:192.168.1.1", "device_namespace:default", "type:application"},
				},
			},
		},
		{
			name: "Director with invalid IP address",
			director: &client.DirectorStatus{
				SystemUpTime: client.DirectorSystemUpTime{
					ApplicationUpTime: "7278 days 16 hours 0 minutes 30 seconds",
					SysProcUptime:     "10 days 2 hours 3 minutes 4 seconds",
				},
			},
			expectedMetrics: []expectedMetric{}, // No metrics should be sent if IP address is invalid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender("testID")
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			s := NewSender(mockSender, "default")
			s.SendDirectorUptimeMetrics(tt.director)

			for _, metric := range tt.expectedMetrics {
				mockSender.AssertMetric(t, "Gauge", metric.name, metric.value, "", metric.tags)
			}

			// Verify no unexpected metrics were sent
			mockSender.AssertNumberOfCalls(t, "Gauge", len(tt.expectedMetrics))
		})
	}
}

func TestParseDiskUsage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []partition
		errMsg   string
	}{
		{
			name:  "Valid single partition",
			input: "partition=root,size=50GB,free=10GB,usedRatio=20.0",
			expected: []partition{
				{
					Name:      "root",
					Size:      50e9, // 50GB
					Free:      10e9, // 10GB
					UsedRatio: 20.0,
				},
			},
		},
		{
			name:  "Valid multiple partitions",
			input: "partition=root,size=50GB,free=10GB,usedRatio=20.0;partition=opt,size=30GB,free=15GB,usedRatio=50.0",
			expected: []partition{
				{
					Name:      "root",
					Size:      50e9, // 50GB
					Free:      10e9, // 10GB
					UsedRatio: 20.0,
				},
				{
					Name:      "opt",
					Size:      30e9, // 30GB
					Free:      15e9, // 15GB
					UsedRatio: 50.0,
				},
			},
		},
		{
			name:  "Valid partition with different size units",
			input: "partition=root,size=1024MiB,free=512MiB,usedRatio=50.0",
			expected: []partition{
				{
					Name:      "root",
					Size:      1024 * 1024 * 1024, // 1024MiB in bytes
					Free:      512 * 1024 * 1024,  // 512MiB in bytes
					UsedRatio: 50.0,
				},
			},
		},
		{
			name:   "Empty input",
			input:  "",
			errMsg: "failed to parse diskUsage string",
		},
		{
			name:   "Random string input",
			input:  "invalid",
			errMsg: "no valid partitions found in diskUsage string",
		},
		{
			name:   "Invalid key-value format",
			input:  "partition=root,size",
			errMsg: "no valid partitions found in diskUsage string",
		},
		{
			name:   "Missing partition name",
			input:  "partition=,size=50GB,free=10GB,usedRatio=20.0",
			errMsg: "no valid partitions found in diskUsage string",
		},
		{
			name:   "Invalid size format",
			input:  "partition=root,size=invalid,free=10GB,usedRatio=20.0",
			errMsg: "no valid partitions found in diskUsage string",
		},
		{
			name:   "Invalid free format",
			input:  "partition=root,size=50GB,free=invalid,usedRatio=20.0",
			errMsg: "no valid partitions found in diskUsage string",
		},
		{
			name:   "Invalid usedRatio format",
			input:  "partition=root,size=50GB,free=10GB,usedRatio=invalid",
			errMsg: "no valid partitions found in diskUsage string",
		},
		{
			name:  "Partial success with some invalid partitions",
			input: "partition=root,size=50GB,free=10GB,usedRatio=20.0;partition=opt,size=invalid,free=15GB,usedRatio=50.0",
			expected: []partition{
				{
					Name:      "root",
					Size:      50e9, // 50GB
					Free:      10e9, // 10GB
					UsedRatio: 20.0,
				},
			},
			errMsg: "failed to parse disk size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDiskUsage(tt.input)
			if err != nil {
				if tt.errMsg == "" {
					t.Errorf("Unexpected error parsing disk usage %s: %v", tt.input, err)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error message to contain %q, got %q", tt.errMsg, err.Error())
				}
				return
			}

			if tt.errMsg != "" {
				t.Errorf("Expected error containing %q but got none", tt.errMsg)
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d partitions but got %d", len(tt.expected), len(result))
				return
			}

			for i, partition := range tt.expected {
				if result[i].Name != partition.Name {
					t.Errorf("Partition %d: expected Name %s but got %s", i, partition.Name, result[i].Name)
				}
				if result[i].Size != partition.Size {
					t.Errorf("Partition %d: expected Size %f but got %f", i, partition.Size, result[i].Size)
				}
				if result[i].Free != partition.Free {
					t.Errorf("Partition %d: expected Free %f but got %f", i, partition.Free, result[i].Free)
				}
				if result[i].UsedRatio != partition.UsedRatio {
					t.Errorf("Partition %d: expected UsedRatio %f but got %f", i, partition.UsedRatio, result[i].UsedRatio)
				}
			}
		})
	}
}

func TestSendDeviceStatusMetrics(t *testing.T) {
	tts := []struct {
		name            string
		input           map[string]float64
		expectedMetrics []expectedMetric
	}{
		{
			name: "single reachable device",
			input: map[string]float64{
				"192.168.1.2": 1.0,
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "device.reachable",
					value: 1.0,
					tags:  []string{"device_ip:192.168.1.2", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "device.unreachable",
					value: 0.0,
					tags:  []string{"device_ip:192.168.1.2", "device_namespace:default"},
				},
			},
		},
		{
			name: "single unreachable device",
			input: map[string]float64{
				"192.168.1.2": 0.0,
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "device.reachable",
					value: 0.0,
					tags:  []string{"device_ip:192.168.1.2", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "device.unreachable",
					value: 1.0,
					tags:  []string{"device_ip:192.168.1.2", "device_namespace:default"},
				},
			},
		},
		{
			name: "multiple devices",
			input: map[string]float64{
				"192.168.1.2": 0.0,
				"10.0.0.1":    1.0,
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "device.reachable",
					value: 0.0,
					tags:  []string{"device_ip:192.168.1.2", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "device.unreachable",
					value: 1.0,
					tags:  []string{"device_ip:192.168.1.2", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "device.reachable",
					value: 1.0,
					tags:  []string{"device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "device.unreachable",
					value: 0.0,
					tags:  []string{"device_ip:10.0.0.1", "device_namespace:default"},
				},
			},
		},
	}

	for _, tt := range tts {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender("testID")
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			s := NewSender(mockSender, "default")
			s.SendDeviceStatusMetrics(tt.input)

			for _, metric := range tt.expectedMetrics {
				mockSender.AssertMetric(t, "Gauge", metric.name, metric.value, "", metric.tags)
			}

			// Verify no unexpected metrics were sent
			mockSender.AssertNumberOfCalls(t, "Gauge", len(tt.expectedMetrics))
		})
	}
}

func TestSendInterfaceStatus(t *testing.T) {
	tts := []struct {
		name                string
		interfaces          []client.Interface
		deviceNameToIPMap   map[string]string
		expectedMetrics     []expectedMetric
		expectedLogWarnings []string
	}{
		{
			name: "single interface",
			interfaces: []client.Interface{
				{
					DeviceName: "device1",
					TenantName: "tenant1",
					Name:       "eth0",
					Type:       "ethernet",
					VRF:        "default",
				},
			},
			deviceNameToIPMap: map[string]string{
				"device1": "192.168.1.1",
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "interface.status",
					value: 1.0,
					tags: []string{
						"device_ip:192.168.1.1",
						"device_namespace:default",
						"interface:eth0",
						"tenant:tenant1",
						"device_name:device1",
					},
				},
			},
		},
		{
			name: "multiple interfaces",
			interfaces: []client.Interface{
				{
					DeviceName: "device1",
					TenantName: "tenant1",
					Name:       "eth0",
					Type:       "ethernet",
					VRF:        "default",
				},
				{
					DeviceName: "device2",
					TenantName: "tenant2",
					Name:       "eth1",
					Type:       "ethernet",
					VRF:        "vrf1",
				},
			},
			deviceNameToIPMap: map[string]string{
				"device1": "192.168.1.1",
				"device2": "192.168.1.2",
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "interface.status",
					value: 1.0,
					tags: []string{
						"device_ip:192.168.1.1",
						"device_namespace:default",
						"interface:eth0",
						"tenant:tenant1",
						"device_name:device1",
					},
				},
				{
					name:  versaMetricPrefix + "interface.status",
					value: 1.0,
					tags: []string{
						"device_ip:192.168.1.2",
						"device_namespace:default",
						"interface:eth1",
						"tenant:tenant2",
						"device_name:device2",
					},
				},
			},
		},
		{
			name: "interface with missing device IP",
			interfaces: []client.Interface{
				{
					DeviceName: "device1",
					TenantName: "tenant1",
					Name:       "eth0",
					Type:       "ethernet",
					VRF:        "default",
				},
			},
			deviceNameToIPMap: map[string]string{
				"device2": "192.168.1.2", // device1 is not in the map
			},
			expectedMetrics:     []expectedMetric{},
			expectedLogWarnings: []string{"device IP not found for device device1, skipping interface status"},
		},
	}

	for _, tt := range tts {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender("testID")
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			s := NewSender(mockSender, "default")
			s.SendInterfaceStatus(tt.interfaces, tt.deviceNameToIPMap)

			for _, metric := range tt.expectedMetrics {
				mockSender.AssertMetric(t, "Gauge", metric.name, metric.value, "", metric.tags)
			}

			// Verify no unexpected metrics were sent
			mockSender.AssertNumberOfCalls(t, "Gauge", len(tt.expectedMetrics))
		})
	}
}

func TestSendLinkStatusMetrics(t *testing.T) {
	tests := []struct {
		name              string
		linkStatusMetrics []client.LinkStatusMetrics
		deviceNameToIDMap map[string]string
		expectedMetrics   []expectedMetric
	}{
		{
			name: "Single link status metric with device mapping",
			linkStatusMetrics: []client.LinkStatusMetrics{
				{
					DrillKey:      "test-branch-2B,INET-1",
					Site:          "test-branch-2B",
					AccessCircuit: "INET-1",
					Availability:  98.5,
				},
			},
			deviceNameToIDMap: map[string]string{
				"test-branch-2B": "192.168.1.1",
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "link.availability",
					value: 98.5,
					tags:  []string{"site:test-branch-2B", "access_circuit:INET-1", "device_ip:192.168.1.1", "device_namespace:default"},
				},
			},
		},
		{
			name: "Single link status metric without device mapping",
			linkStatusMetrics: []client.LinkStatusMetrics{
				{
					DrillKey:      "test-branch-3C,INET-2",
					Site:          "test-branch-3C",
					AccessCircuit: "INET-2",
					Availability:  95.0,
				},
			},
			deviceNameToIDMap: map[string]string{},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "link.availability",
					value: 95.0,
					tags:  []string{"site:test-branch-3C", "access_circuit:INET-2"},
				},
			},
		},
		{
			name: "Multiple link status metrics with mixed device mapping",
			linkStatusMetrics: []client.LinkStatusMetrics{
				{
					DrillKey:      "branch-1,MPLS-1",
					Site:          "branch-1",
					AccessCircuit: "MPLS-1",
					Availability:  99.9,
				},
				{
					DrillKey:      "branch-2,INET-1",
					Site:          "branch-2",
					AccessCircuit: "INET-1",
					Availability:  97.2,
				},
			},
			deviceNameToIDMap: map[string]string{
				"branch-1": "10.0.0.1",
				// branch-2 is intentionally missing to test the no mapping case
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "link.availability",
					value: 99.9,
					tags:  []string{"site:branch-1", "access_circuit:MPLS-1", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "link.availability",
					value: 97.2,
					tags:  []string{"site:branch-2", "access_circuit:INET-1"},
				},
			},
		},
		{
			name:              "Empty link status metrics",
			linkStatusMetrics: []client.LinkStatusMetrics{},
			deviceNameToIDMap: map[string]string{},
			expectedMetrics:   []expectedMetric{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender("testID")
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			s := NewSender(mockSender, "default")
			s.SendLinkStatusMetrics(tt.linkStatusMetrics, tt.deviceNameToIDMap)

			for _, metric := range tt.expectedMetrics {
				mockSender.AssertMetric(t, "Gauge", metric.name, metric.value, "", metric.tags)
			}

			// Verify no unexpected metrics were sent
			mockSender.AssertNumberOfCalls(t, "Gauge", len(tt.expectedMetrics))
		})
	}
}

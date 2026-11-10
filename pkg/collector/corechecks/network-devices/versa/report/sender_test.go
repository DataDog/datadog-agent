// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package report

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
			mockSender := mocksender.NewMockSender(t, "testID")
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
			mockSender := mocksender.NewMockSender(t, "testID")
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
			mockSender := mocksender.NewMockSender(t, "testID")
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
			mockSender := mocksender.NewMockSender(t, "testID")
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
					DeviceName:    "device1",
					TenantName:    "tenant1",
					Name:          "eth0",
					Type:          "ethernet",
					VRF:           "default",
					IfAdminStatus: "up",
					IfOperStatus:  "down",
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
						"admin_status:up",
						"oper_status:down",
					},
				},
			},
		},
		{
			name: "multiple interfaces",
			interfaces: []client.Interface{
				{
					DeviceName:    "device1",
					TenantName:    "tenant1",
					Name:          "eth0",
					Type:          "ethernet",
					VRF:           "default",
					IfAdminStatus: "off",
					IfOperStatus:  "alerting",
				},
				{
					DeviceName:    "device2",
					TenantName:    "tenant2",
					Name:          "eth1",
					Type:          "ethernet",
					VRF:           "vrf1",
					IfAdminStatus: "testing",
					IfOperStatus:  "notPresent",
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
						"admin_status:off",
						"oper_status:alerting",
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
						"admin_status:testing",
						"oper_status:notPresent", // TODO: fix casing
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
			mockSender := mocksender.NewMockSender(t, "testID")
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
				"test-branch-2b": "192.168.1.1", // lowercased: device-name map keys are normalized
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
			mockSender := mocksender.NewMockSender(t, "testID")
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

func TestSendApplicationsByApplianceMetrics(t *testing.T) {
	tests := []struct {
		name                   string
		appsByApplianceMetrics []client.ApplicationsByApplianceMetrics
		deviceNameToIDMap      map[string]string
		expectedMetrics        []expectedMetric
	}{
		{
			name: "Single applications by appliance metric with device mapping",
			appsByApplianceMetrics: []client.ApplicationsByApplianceMetrics{
				{
					DrillKey:    "test-branch-2B,HTTP",
					Site:        "test-branch-2B",
					AppID:       "HTTP",
					Sessions:    50.0,
					VolumeTx:    1024000.0,
					VolumeRx:    512000.0,
					BandwidthTx: 8192.0,
					BandwidthRx: 4096.0,
					Bandwidth:   12288.0,
				},
			},
			deviceNameToIDMap: map[string]string{
				"test-branch-2b": "192.168.1.1", // lowercased: device-name map keys are normalized
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "app.sessions",
					value: 50.0,
					tags:  []string{"site:test-branch-2B", "app_id:HTTP", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "app.volume_tx",
					value: 1024000.0,
					tags:  []string{"site:test-branch-2B", "app_id:HTTP", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "app.volume_rx",
					value: 512000.0,
					tags:  []string{"site:test-branch-2B", "app_id:HTTP", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "app.bandwidth_tx",
					value: 8192.0,
					tags:  []string{"site:test-branch-2B", "app_id:HTTP", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "app.bandwidth_rx",
					value: 4096.0,
					tags:  []string{"site:test-branch-2B", "app_id:HTTP", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "app.bandwidth",
					value: 12288.0,
					tags:  []string{"site:test-branch-2B", "app_id:HTTP", "device_ip:192.168.1.1", "device_namespace:default"},
				},
			},
		},
		{
			name: "Multiple applications by appliance metrics with mixed device mapping",
			appsByApplianceMetrics: []client.ApplicationsByApplianceMetrics{
				{
					DrillKey:    "branch-1,HTTPS",
					Site:        "branch-1",
					AppID:       "HTTPS",
					Sessions:    100.0,
					VolumeTx:    2048000.0,
					VolumeRx:    1024000.0,
					BandwidthTx: 16384.0,
					BandwidthRx: 8192.0,
					Bandwidth:   24576.0,
				},
				{
					DrillKey:    "branch-2,SSH",
					Site:        "branch-2",
					AppID:       "SSH",
					Sessions:    25.0,
					VolumeTx:    256000.0,
					VolumeRx:    128000.0,
					BandwidthTx: 2048.0,
					BandwidthRx: 1024.0,
					Bandwidth:   3072.0,
				},
			},
			deviceNameToIDMap: map[string]string{
				"branch-1": "10.0.0.1",
				// branch-2 is intentionally missing to test the no mapping case
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "app.sessions",
					value: 100.0,
					tags:  []string{"site:branch-1", "app_id:HTTPS", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "app.volume_tx",
					value: 2048000.0,
					tags:  []string{"site:branch-1", "app_id:HTTPS", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "app.volume_rx",
					value: 1024000.0,
					tags:  []string{"site:branch-1", "app_id:HTTPS", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "app.bandwidth_tx",
					value: 16384.0,
					tags:  []string{"site:branch-1", "app_id:HTTPS", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "app.bandwidth_rx",
					value: 8192.0,
					tags:  []string{"site:branch-1", "app_id:HTTPS", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "app.bandwidth",
					value: 24576.0,
					tags:  []string{"site:branch-1", "app_id:HTTPS", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "app.sessions",
					value: 25.0,
					tags:  []string{"site:branch-2", "app_id:SSH"},
				},
				{
					name:  versaMetricPrefix + "app.volume_tx",
					value: 256000.0,
					tags:  []string{"site:branch-2", "app_id:SSH"},
				},
				{
					name:  versaMetricPrefix + "app.volume_rx",
					value: 128000.0,
					tags:  []string{"site:branch-2", "app_id:SSH"},
				},
				{
					name:  versaMetricPrefix + "app.bandwidth_tx",
					value: 2048.0,
					tags:  []string{"site:branch-2", "app_id:SSH"},
				},
				{
					name:  versaMetricPrefix + "app.bandwidth_rx",
					value: 1024.0,
					tags:  []string{"site:branch-2", "app_id:SSH"},
				},
				{
					name:  versaMetricPrefix + "app.bandwidth",
					value: 3072.0,
					tags:  []string{"site:branch-2", "app_id:SSH"},
				},
			},
		},
		{
			name:                   "Empty applications by appliance metrics",
			appsByApplianceMetrics: []client.ApplicationsByApplianceMetrics{},
			deviceNameToIDMap:      map[string]string{},
			expectedMetrics:        []expectedMetric{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender(t, "testID")
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			s := NewSender(mockSender, "default")
			s.SendApplicationsByApplianceMetrics(tt.appsByApplianceMetrics, tt.deviceNameToIDMap)

			for _, metric := range tt.expectedMetrics {
				mockSender.AssertMetric(t, "Gauge", metric.name, metric.value, "", metric.tags)
			}

			// Verify no unexpected metrics were sent
			mockSender.AssertNumberOfCalls(t, "Gauge", len(tt.expectedMetrics))
		})
	}
}

func TestSendTopUserMetrics(t *testing.T) {
	tests := []struct {
		name              string
		topUserMetrics    []client.TopUserMetrics
		deviceNameToIDMap map[string]string
		expectedMetrics   []expectedMetric
	}{
		{
			name: "Single user and appliance with device mapping",
			topUserMetrics: []client.TopUserMetrics{
				{
					DrillKey:    "test-branch-2B,testUser1",
					Site:        "test-branch-2B",
					User:        "testUser1",
					Sessions:    50.0,
					VolumeTx:    1024000.0,
					VolumeRx:    512000.0,
					BandwidthTx: 8192.0,
					BandwidthRx: 4096.0,
					Bandwidth:   12288.0,
				},
			},
			deviceNameToIDMap: map[string]string{
				"test-branch-2b": "192.168.1.1", // lowercased: device-name map keys are normalized
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "user.sessions",
					value: 50.0,
					tags:  []string{"site:test-branch-2B", "user:testUser1", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.volume_tx",
					value: 1024000.0,
					tags:  []string{"site:test-branch-2B", "user:testUser1", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.volume_rx",
					value: 512000.0,
					tags:  []string{"site:test-branch-2B", "user:testUser1", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.bandwidth_tx",
					value: 8192.0,
					tags:  []string{"site:test-branch-2B", "user:testUser1", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.bandwidth_rx",
					value: 4096.0,
					tags:  []string{"site:test-branch-2B", "user:testUser1", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.bandwidth",
					value: 12288.0,
					tags:  []string{"site:test-branch-2B", "user:testUser1", "device_ip:192.168.1.1", "device_namespace:default"},
				},
			},
		},
		{
			name: "Multiple users single appliance with device mapping",
			topUserMetrics: []client.TopUserMetrics{
				{
					DrillKey:    "branch-1,testUser2",
					Site:        "branch-1",
					User:        "testUser2",
					Sessions:    100.0,
					VolumeTx:    2048000.0,
					VolumeRx:    1024000.0,
					BandwidthTx: 16384.0,
					BandwidthRx: 8192.0,
					Bandwidth:   24576.0,
				},
				{
					DrillKey:    "branch-1,testUser3",
					Site:        "branch-1",
					User:        "testUser3",
					Sessions:    25.0,
					VolumeTx:    256000.0,
					VolumeRx:    128000.0,
					BandwidthTx: 2048.0,
					BandwidthRx: 1024.0,
					Bandwidth:   3072.0,
				},
			},
			deviceNameToIDMap: map[string]string{
				"branch-1": "10.0.0.1",
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "user.sessions",
					value: 100.0,
					tags:  []string{"site:branch-1", "user:testUser2", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.volume_tx",
					value: 2048000.0,
					tags:  []string{"site:branch-1", "user:testUser2", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.volume_rx",
					value: 1024000.0,
					tags:  []string{"site:branch-1", "user:testUser2", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.bandwidth_tx",
					value: 16384.0,
					tags:  []string{"site:branch-1", "user:testUser2", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.bandwidth_rx",
					value: 8192.0,
					tags:  []string{"site:branch-1", "user:testUser2", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.bandwidth",
					value: 24576.0,
					tags:  []string{"site:branch-1", "user:testUser2", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.sessions",
					value: 25.0,
					tags:  []string{"site:branch-1", "user:testUser3", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.volume_tx",
					value: 256000.0,
					tags:  []string{"site:branch-1", "user:testUser3", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.volume_rx",
					value: 128000.0,
					tags:  []string{"site:branch-1", "user:testUser3", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.bandwidth_tx",
					value: 2048.0,
					tags:  []string{"site:branch-1", "user:testUser3", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.bandwidth_rx",
					value: 1024.0,
					tags:  []string{"site:branch-1", "user:testUser3", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "user.bandwidth",
					value: 3072.0,
					tags:  []string{"site:branch-1", "user:testUser3", "device_ip:10.0.0.1", "device_namespace:default"},
				},
			},
		},
		{
			name:              "Empty top user metrics",
			topUserMetrics:    []client.TopUserMetrics{},
			deviceNameToIDMap: map[string]string{},
			expectedMetrics:   []expectedMetric{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender(t, "testID")
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			s := NewSender(mockSender, "default")
			s.SendTopUserMetrics(tt.topUserMetrics, tt.deviceNameToIDMap)

			for _, metric := range tt.expectedMetrics {
				mockSender.AssertMetric(t, "Gauge", metric.name, metric.value, "", metric.tags)
			}

			// Verify no unexpected metrics were sent
			mockSender.AssertNumberOfCalls(t, "Gauge", len(tt.expectedMetrics))
		})
	}
}

func TestSendAnalyticsInterfaceMetrics(t *testing.T) {
	tests := []struct {
		name                      string
		analyticsInterfaceMetrics []client.AnalyticsInterfaceMetrics
		deviceNameToIDMap         map[string]string
		expectedMetrics           []expectedMetric
	}{
		{
			name: "Single analytics interface metric with device mapping",
			analyticsInterfaceMetrics: []client.AnalyticsInterfaceMetrics{
				{
					DrillKey:    "test-branch-2B,INET-1,ge-0/0/1",
					Site:        "test-branch-2B",
					AccessCkt:   "INET-1",
					Interface:   "ge-0/0/1",
					RxUtil:      25.5,
					TxUtil:      18.3,
					VolumeRx:    1024000.0,
					VolumeTx:    768000.0,
					Volume:      1792000.0,
					BandwidthRx: 8192.0,
					BandwidthTx: 6144.0,
					Bandwidth:   14336.0,
				},
			},
			deviceNameToIDMap: map[string]string{
				"test-branch-2b": "192.168.1.1", // lowercased: device-name map keys are normalized
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "interface.rx_util",
					value: 25.5,
					tags:  []string{"site:test-branch-2B", "access_circuit:INET-1", "interface:ge-0/0/1", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "interface.tx_util",
					value: 18.3,
					tags:  []string{"site:test-branch-2B", "access_circuit:INET-1", "interface:ge-0/0/1", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "interface.volume_rx",
					value: 1024000.0,
					tags:  []string{"site:test-branch-2B", "access_circuit:INET-1", "interface:ge-0/0/1", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "interface.volume_tx",
					value: 768000.0,
					tags:  []string{"site:test-branch-2B", "access_circuit:INET-1", "interface:ge-0/0/1", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "interface.volume",
					value: 1792000.0,
					tags:  []string{"site:test-branch-2B", "access_circuit:INET-1", "interface:ge-0/0/1", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "interface.bandwidth_rx",
					value: 8192.0,
					tags:  []string{"site:test-branch-2B", "access_circuit:INET-1", "interface:ge-0/0/1", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "interface.bandwidth_tx",
					value: 6144.0,
					tags:  []string{"site:test-branch-2B", "access_circuit:INET-1", "interface:ge-0/0/1", "device_ip:192.168.1.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "interface.bandwidth",
					value: 14336.0,
					tags:  []string{"site:test-branch-2B", "access_circuit:INET-1", "interface:ge-0/0/1", "device_ip:192.168.1.1", "device_namespace:default"},
				},
			},
		},
		{
			name: "Multiple analytics interface metrics with mixed device mapping",
			analyticsInterfaceMetrics: []client.AnalyticsInterfaceMetrics{
				{
					DrillKey:    "branch-1,WAN-1,ge-0/0/0",
					Site:        "branch-1",
					AccessCkt:   "WAN-1",
					Interface:   "ge-0/0/0",
					RxUtil:      45.2,
					TxUtil:      32.8,
					VolumeRx:    2048000.0,
					VolumeTx:    1536000.0,
					Volume:      3584000.0,
					BandwidthRx: 16384.0,
					BandwidthTx: 12288.0,
					Bandwidth:   28672.0,
				},
				{
					DrillKey:    "branch-2,LAN-1,ge-0/0/2",
					Site:        "branch-2",
					AccessCkt:   "LAN-1",
					Interface:   "ge-0/0/2",
					RxUtil:      15.7,
					TxUtil:      8.9,
					VolumeRx:    512000.0,
					VolumeTx:    256000.0,
					Volume:      768000.0,
					BandwidthRx: 4096.0,
					BandwidthTx: 2048.0,
					Bandwidth:   6144.0,
				},
			},
			deviceNameToIDMap: map[string]string{
				"branch-1": "10.0.0.1",
				// branch-2 is intentionally missing to test the no mapping case
			},
			expectedMetrics: []expectedMetric{
				{
					name:  versaMetricPrefix + "interface.rx_util",
					value: 45.2,
					tags:  []string{"site:branch-1", "access_circuit:WAN-1", "interface:ge-0/0/0", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "interface.tx_util",
					value: 32.8,
					tags:  []string{"site:branch-1", "access_circuit:WAN-1", "interface:ge-0/0/0", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "interface.volume_rx",
					value: 2048000.0,
					tags:  []string{"site:branch-1", "access_circuit:WAN-1", "interface:ge-0/0/0", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "interface.volume_tx",
					value: 1536000.0,
					tags:  []string{"site:branch-1", "access_circuit:WAN-1", "interface:ge-0/0/0", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "interface.volume",
					value: 3584000.0,
					tags:  []string{"site:branch-1", "access_circuit:WAN-1", "interface:ge-0/0/0", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "interface.bandwidth_rx",
					value: 16384.0,
					tags:  []string{"site:branch-1", "access_circuit:WAN-1", "interface:ge-0/0/0", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "interface.bandwidth_tx",
					value: 12288.0,
					tags:  []string{"site:branch-1", "access_circuit:WAN-1", "interface:ge-0/0/0", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "interface.bandwidth",
					value: 28672.0,
					tags:  []string{"site:branch-1", "access_circuit:WAN-1", "interface:ge-0/0/0", "device_ip:10.0.0.1", "device_namespace:default"},
				},
				{
					name:  versaMetricPrefix + "interface.rx_util",
					value: 15.7,
					tags:  []string{"site:branch-2", "access_circuit:LAN-1", "interface:ge-0/0/2"},
				},
				{
					name:  versaMetricPrefix + "interface.tx_util",
					value: 8.9,
					tags:  []string{"site:branch-2", "access_circuit:LAN-1", "interface:ge-0/0/2"},
				},
				{
					name:  versaMetricPrefix + "interface.volume_rx",
					value: 512000.0,
					tags:  []string{"site:branch-2", "access_circuit:LAN-1", "interface:ge-0/0/2"},
				},
				{
					name:  versaMetricPrefix + "interface.volume_tx",
					value: 256000.0,
					tags:  []string{"site:branch-2", "access_circuit:LAN-1", "interface:ge-0/0/2"},
				},
				{
					name:  versaMetricPrefix + "interface.volume",
					value: 768000.0,
					tags:  []string{"site:branch-2", "access_circuit:LAN-1", "interface:ge-0/0/2"},
				},
				{
					name:  versaMetricPrefix + "interface.bandwidth_rx",
					value: 4096.0,
					tags:  []string{"site:branch-2", "access_circuit:LAN-1", "interface:ge-0/0/2"},
				},
				{
					name:  versaMetricPrefix + "interface.bandwidth_tx",
					value: 2048.0,
					tags:  []string{"site:branch-2", "access_circuit:LAN-1", "interface:ge-0/0/2"},
				},
				{
					name:  versaMetricPrefix + "interface.bandwidth",
					value: 6144.0,
					tags:  []string{"site:branch-2", "access_circuit:LAN-1", "interface:ge-0/0/2"},
				},
			},
		},
		{
			name:                      "Empty analytics interface metrics",
			analyticsInterfaceMetrics: []client.AnalyticsInterfaceMetrics{},
			deviceNameToIDMap:         map[string]string{},
			expectedMetrics:           []expectedMetric{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender(t, "testID")
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			s := NewSender(mockSender, "default")
			s.SendAnalyticsInterfaceMetrics(tt.analyticsInterfaceMetrics, tt.deviceNameToIDMap)

			for _, metric := range tt.expectedMetrics {
				mockSender.AssertMetric(t, "Gauge", metric.name, metric.value, "", metric.tags)
			}

			// Verify no unexpected metrics were sent
			mockSender.AssertNumberOfCalls(t, "Gauge", len(tt.expectedMetrics))
		})
	}
}

// TestSendAnalyticsInterfaceMetricsCaseInsensitiveDeviceLookup is a regression test for a defect
// where an Analytics result whose site name differed in case from the Director's appliance name
// passed device filtering (which matches case-insensitively) but then missed the device-tag
// lookup (which was case-sensitive), so the metric was emitted with no device_id. Versa reports
// the same device with inconsistent casing, so the device-name map is keyed lowercase and every
// lookup normalizes.
func TestSendAnalyticsInterfaceMetricsCaseInsensitiveDeviceLookup(t *testing.T) {
	mockSender := mocksender.NewMockSender(t, "testID")
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	// The Director reported this appliance as "Branch-1"; Analytics reports the site as "BRANCH-1"
	deviceNameToIDMap := map[string]string{"branch-1": "10.0.0.1"}
	analyticsInterfaceMetrics := []client.AnalyticsInterfaceMetrics{
		{
			Site:      "BRANCH-1",
			AccessCkt: "WAN-1",
			Interface: "ge-0/0/0",
			RxUtil:    15.7,
		},
	}

	s := NewSender(mockSender, "default")
	s.SendAnalyticsInterfaceMetrics(analyticsInterfaceMetrics, deviceNameToIDMap)

	mockSender.AssertMetric(t, "Gauge", versaMetricPrefix+"interface.rx_util", 15.7, "",
		[]string{"site:BRANCH-1", "access_circuit:WAN-1", "interface:ge-0/0/0", "device_ip:10.0.0.1", "device_namespace:default"})
}

func TestSendSLAMetricsTableData(t *testing.T) {
	mockSender := mocksender.NewMockSender(t, "testID")
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	s := NewSender(mockSender, "default")
	s.SendSLAMetrics("datadog", []client.SLAMetrics{
		{
			DrillKey:            "branch-1,hub-1,INET-1,INET-1,fc_nc",
			LocalSite:           "branch-1",
			RemoteSite:          "hub-1",
			LocalAccessCircuit:  "INET-1",
			RemoteAccessCircuit: "INET-1",
			ForwardingClass:     "fc_nc",
			Delay:               101,
		},
	}, map[string]string{"branch-1": "192.168.1.1"})

	tags := []string{
		"local_site:branch-1",
		"remote_site:hub-1",
		"local_access_circuit:INET-1",
		"remote_access_circuit:INET-1",
		"forwarding_class:fc_nc",
		"device_ip:192.168.1.1",
		"device_namespace:default",
	}
	// A tableData row carries no bucket timestamp, so it is still submitted at collection
	// time as a plain gauge.
	mockSender.AssertMetric(t, "Gauge", versaMetricPrefix+"sla.delay", 101, "", tags)
	mockSender.AssertNumberOfCalls(t, "Gauge", 6)
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 0)
}

func TestSendSLAMetricsTimeseries(t *testing.T) {
	TimeNow = mockTimeNow
	mockSender := mocksender.NewMockSender(t, "testID")
	mockSender.On("GaugeWithTimestamp", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	buckets := []client.SLAMetrics{
		{
			DrillKey:            "branch-1,hub-1,INET-1,INET-1,fc_nc",
			LocalSite:           "branch-1",
			RemoteSite:          "hub-1",
			LocalAccessCircuit:  "INET-1",
			RemoteAccessCircuit: "INET-1",
			ForwardingClass:     "fc_nc",
			Delay:               99,
			Timestamp:           1704067200000,
		},
		{
			DrillKey:            "branch-1,hub-1,INET-1,INET-1,fc_nc",
			LocalSite:           "branch-1",
			RemoteSite:          "hub-1",
			LocalAccessCircuit:  "INET-1",
			RemoteAccessCircuit: "INET-1",
			ForwardingClass:     "fc_nc",
			Delay:               140,
			Timestamp:           1704068100000,
		},
	}

	s := NewSender(mockSender, "default")
	s.SendSLAMetrics("datadog", buckets, map[string]string{"branch-1": "192.168.1.1"})

	tags := []string{
		"local_site:branch-1",
		"remote_site:hub-1",
		"local_access_circuit:INET-1",
		"remote_access_circuit:INET-1",
		"forwarding_class:fc_nc",
		"device_ip:192.168.1.1",
		"device_namespace:default",
	}
	// Every bucket keeps its own point rather than collapsing into one value per run.
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", versaMetricPrefix+"sla.delay", 99, "", tags, 1704067200)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", versaMetricPrefix+"sla.delay", 140, "", tags, 1704068100)
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 12)
	mockSender.AssertNumberOfCalls(t, "Gauge", 0)

	// A lookback window longer than the check interval re-reports buckets that have
	// already been sent, so a second run of the same buckets emits nothing.
	s.SendSLAMetrics("datadog", buckets, map[string]string{"branch-1": "192.168.1.1"})
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 12)

	// A newer bucket for the same path still goes out.
	s.SendSLAMetrics("datadog", []client.SLAMetrics{{
		DrillKey:            "branch-1,hub-1,INET-1,INET-1,fc_nc",
		LocalSite:           "branch-1",
		RemoteSite:          "hub-1",
		LocalAccessCircuit:  "INET-1",
		RemoteAccessCircuit: "INET-1",
		ForwardingClass:     "fc_nc",
		Delay:               98,
		Timestamp:           1704069000000,
	}}, map[string]string{"branch-1": "192.168.1.1"})
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", versaMetricPrefix+"sla.delay", 98, "", tags, 1704069000)
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 18)
}

func TestSendCircuitQoSMetrics(t *testing.T) {
	mockSender := mocksender.NewMockSender(t, "testID")
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	s := NewSender(mockSender, "default")
	s.SendCircuitQoSMetrics("datadog", []client.CircuitQoSMetrics{
		{
			DrillKey:            "branch-1,INET-1",
			Site:                "branch-1",
			AccessCircuit:       "INET-1",
			BestEffortTx:        1000,
			BestEffortTxDrop:    10,
			BestEffortBandwidth: 8000,
			VolumeTx:            1850,
			TotalDrop:           10,
			PercentDrop:         0.54,
			Bandwidth:           14800,
		},
	}, map[string]string{"branch-1": "192.168.1.1"})

	tags := []string{
		"site:branch-1",
		"access_circuit:INET-1",
		"device_ip:192.168.1.1",
		"device_namespace:default",
	}
	mockSender.AssertMetric(t, "Gauge", versaMetricPrefix+"circuit_qos.best_effort_tx", 1000, "", tags)
	mockSender.AssertMetric(t, "Gauge", versaMetricPrefix+"circuit_qos.best_effort_tx_drop", 10, "", tags)
	mockSender.AssertMetric(t, "Gauge", versaMetricPrefix+"circuit_qos.best_effort_bandwidth", 8000, "", tags)
	mockSender.AssertMetric(t, "Gauge", versaMetricPrefix+"circuit_qos.volume_tx", 1850, "", tags)
	mockSender.AssertMetric(t, "Gauge", versaMetricPrefix+"circuit_qos.total_drop", 10, "", tags)
	mockSender.AssertMetric(t, "Gauge", versaMetricPrefix+"circuit_qos.percent_drop", 0.54, "", tags)
	mockSender.AssertMetric(t, "Gauge", versaMetricPrefix+"circuit_qos.bandwidth", 14800, "", tags)
	mockSender.AssertNumberOfCalls(t, "Gauge", 16)
}

// TestCircuitQoSMetricsEndToEnd runs the circuit QoS fixture through the real client,
// parser and sender, so a column added to the query but not to the parser (or the other
// way round) shows up as a missing or misplaced metric.
func TestCircuitQoSMetricsEndToEnd(t *testing.T) {
	mockAPI := client.SetupMockAPIServer()
	defer mockAPI.Close()
	// Record the metrics the client asks for, in order, before handing the request to
	// the mock API, so the column order the parser relies on is pinned at the source.
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("q"), "cos(") {
			requested = r.URL.Query()["metrics"]
		}
		mockAPI.Config.Handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	c, err := client.NewTestClient(server)
	require.NoError(t, err)

	metrics, err := c.GetCircuitQoSMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Equal(t, []string{
		"betx", "betxdrop", "eftx", "eftxdrop", "aftx", "aftxdrop", "nctx", "nctxdrop",
		"bebandwidth", "efbandwidth", "afbandwidth", "ncbandwidth",
		"volume-tx", "totaldrop", "percentdrop", "bandwidth",
	}, requested)

	mockSender := mocksender.NewMockSender(t, "testID")
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	s := NewSender(mockSender, "default")
	s.SendCircuitQoSMetrics("datadog", metrics, map[string]string{"test-branch-2b": "192.168.1.1"})

	tags := []string{
		"site:test-branch-2B",
		"access_circuit:INET-1",
		"device_ip:192.168.1.1",
		"device_namespace:default",
	}
	expected := map[string]float64{
		"best_effort_tx": 1000, "best_effort_tx_drop": 10,
		"expedited_forward_tx": 500, "expedited_forward_drop": 0,
		"assured_forward_tx": 300, "assured_forward_drop": 0,
		"network_control_tx": 50, "network_control_drop": 0,
		"best_effort_bandwidth": 8000, "expedited_forward_bw": 4000,
		"assured_forward_bw": 2400, "network_control_bw": 400,
		"volume_tx": 1850, "total_drop": 10, "percent_drop": 0.54, "bandwidth": 14800,
	}
	for name, value := range expected {
		mockSender.AssertMetric(t, "Gauge", versaMetricPrefix+"circuit_qos."+name, value, "", tags)
	}
	mockSender.AssertNumberOfCalls(t, "Gauge", len(expected))
}

func slaBuckets(drillKey string, firstMillis int64, count int) []client.SLAMetrics {
	out := make([]client.SLAMetrics, count)
	for i := range out {
		out[i] = client.SLAMetrics{
			DrillKey:  drillKey,
			LocalSite: "site-001", RemoteSite: "hub-001",
			LocalAccessCircuit: "circuit-a", RemoteAccessCircuit: "circuit-a",
			ForwardingClass: "fc_ef",
			Delay:           float64(i),
			Timestamp:       firstMillis + int64(i)*900000,
		}
	}
	return out
}

// TestSendSLAMetricsDedupeAcrossOverlappingWindows models successive runs whose lookback
// windows overlap: each run re-reports most buckets, and only the new ones go out.
func TestSendSLAMetricsDedupeAcrossOverlappingWindows(t *testing.T) {
	TimeNow = mockTimeNow
	mockSender := mocksender.NewMockSender(t, "testID")
	mockSender.On("GaugeWithTimestamp", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	s := NewSender(mockSender, "default")
	const key = "site-001,hub-001,circuit-a,circuit-a,fc_ef"
	const start = int64(1704067200000)

	s.SendSLAMetrics("tenant-1", slaBuckets(key, start, 15), nil)
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 15*6)

	// Window slid by two buckets: thirteen already sent, two new.
	s.SendSLAMetrics("tenant-1", slaBuckets(key, start+2*900000, 15), nil)
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 17*6)

	// Same window again: nothing new.
	s.SendSLAMetrics("tenant-1", slaBuckets(key, start+2*900000, 15), nil)
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 17*6)

	// An older bucket arriving late is still refused.
	s.SendSLAMetrics("tenant-1", slaBuckets(key, start, 1), nil)
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 17*6)
}

// TestSendSLAMetricsDedupeIsTenantKeyed checks that the same drill key under two tenants
// is tracked separately, so the second tenant is not mistaken for a resend.
func TestSendSLAMetricsDedupeIsTenantKeyed(t *testing.T) {
	TimeNow = mockTimeNow
	mockSender := mocksender.NewMockSender(t, "testID")
	mockSender.On("GaugeWithTimestamp", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	s := NewSender(mockSender, "default")
	const key = "site-001,hub-001,circuit-a,circuit-a,fc_ef"
	const start = int64(1704067200000)

	s.SendSLAMetrics("tenant-1", slaBuckets(key, start, 4), nil)
	s.SendSLAMetrics("tenant-2", slaBuckets(key, start, 4), nil)
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 2*4*6)

	s.SendSLAMetrics("tenant-1", slaBuckets(key, start, 4), nil)
	s.SendSLAMetrics("tenant-2", slaBuckets(key, start, 4), nil)
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 2*4*6)
}

func TestSendCircuitQoSMetricsSkipsUnsetPercentDrop(t *testing.T) {
	mockSender := mocksender.NewMockSender(t, "testID")
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	s := NewSender(mockSender, "default")
	s.SendCircuitQoSMetrics("datadog", []client.CircuitQoSMetrics{{
		DrillKey: "site-001,circuit-a", Site: "site-001", AccessCircuit: "circuit-a",
		PercentDropUnset: true,
	}}, nil)

	mockSender.AssertNotCalled(t, "Gauge", versaMetricPrefix+"circuit_qos.percent_drop", mock.Anything, mock.Anything, mock.Anything)
	mockSender.AssertNumberOfCalls(t, "Gauge", 15)
}

func TestSendPathQoSMetricsSkipsUnsetPercentDrop(t *testing.T) {
	mockSender := mocksender.NewMockSender(t, "testID")
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	s := NewSender(mockSender, "default")
	s.SendPathQoSMetrics("datadog", []client.QoSMetrics{{
		DrillKey: "site-001,site-002", LocalSiteName: "site-001", RemoteSiteName: "site-002",
		PercentDropUnset: true,
	}}, nil)

	mockSender.AssertNotCalled(t, "Gauge", versaMetricPrefix+"qos.percent_drop", mock.Anything, mock.Anything, mock.Anything)
	mockSender.AssertNumberOfCalls(t, "Gauge", 15)
}

// pathQoSTimeseriesBody builds a small timeseries pathcos response in the shape Versa
// returns: percentdrop is zero in every bucket even though totaldrop is not.
func pathQoSTimeseriesBody(t *testing.T) []byte {
	buckets := []float64{1704067200000, 1704068100000, 1704069000000}
	values := map[string][]float64{
		"volume-tx":   {2000, 4000, 1000},
		"totaldrop":   {10, 0, 5},
		"percentdrop": {0, 0, 0},
		"bandwidth":   {16000, 32000, 8000},
	}
	var series []map[string]any
	for _, metric := range []string{
		"betx", "betxdrop", "eftx", "eftxdrop", "aftx", "aftxdrop", "nctx", "nctxdrop",
		"bebandwidth", "efbandwidth", "afbandwidth", "ncbandwidth",
		"volume-tx", "totaldrop", "percentdrop", "bandwidth",
	} {
		v, ok := values[metric]
		if !ok {
			v = []float64{0, 0, 0}
		}
		var points [][]float64
		for i, ts := range buckets {
			points = append(points, []float64{ts, v[i]})
		}
		series = append(series, map[string]any{
			"name": "site-001,site-002", "type": "data", "metric": metric, "metricName": metric, "data": points,
		})
	}
	body, err := json.Marshal(map[string]any{"qTime": 1, "data": series})
	require.NoError(t, err)
	return body
}

// TestPathQoSTimeseriesPercentDropEndToEnd runs a timeseries pathcos response through the
// client and sender and checks qos.percent_drop is computed rather than Versa's zero.
func TestPathQoSTimeseriesPercentDropEndToEnd(t *testing.T) {
	mockAPI := client.SetupMockAPIServer()
	defer mockAPI.Close()
	body := pathQoSTimeseriesBody(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("q"), "pathcos(") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
			return
		}
		mockAPI.Config.Handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	c, err := client.NewTestClient(server, client.WithAnalyticsQueryType(client.QueryTypeTimeseries))
	require.NoError(t, err)

	metrics, err := c.GetPathQoSMetrics("datadog")
	require.NoError(t, err)
	// Three buckets, of which the newest is held back as still filling.
	require.Len(t, metrics, 2)

	TimeNow = mockTimeNow
	mockSender := mocksender.NewMockSender(t, "testID")
	mockSender.On("GaugeWithTimestamp", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	s := NewSender(mockSender, "default")
	s.SendPathQoSMetrics("datadog", metrics, nil)

	var percentDrops []float64
	for _, call := range mockSender.Calls {
		if call.Method == "GaugeWithTimestamp" && call.Arguments.String(0) == versaMetricPrefix+"qos.percent_drop" {
			percentDrops = append(percentDrops, call.Arguments.Get(1).(float64))
		}
	}
	// 10/2000 for the bucket with drops, and a real 0 for the bucket without.
	require.Equal(t, []float64{0.005, 0}, percentDrops)
}

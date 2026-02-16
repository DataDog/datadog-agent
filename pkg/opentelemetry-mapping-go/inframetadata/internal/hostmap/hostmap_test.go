// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package hostmap

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pmetric"
	conventions "go.opentelemetry.io/otel/semconv/v1.18.0"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata/internal/testutils"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata/payload"
)

func BuildMetric[N int64 | float64](name string, value N) *pmetric.Metric {
	m := pmetric.NewMetric()
	m.SetEmptyGauge()
	m.SetName(name)
	dp := m.Gauge().DataPoints().AppendEmpty()
	switch any(value).(type) {
	case int64:
		dp.SetIntValue(int64(value))
	case float64:
		dp.SetDoubleValue(float64(value))
	}
	return &m
}

func TestStrSliceField(t *testing.T) {
	tests := []struct {
		attributes  map[string]any
		key         string
		expected    []string
		expectedOk  bool
		expectedErr string
	}{
		{
			attributes:  map[string]any{},
			key:         "nonexistingkey",
			expected:    nil,
			expectedOk:  false,
			expectedErr: "",
		},
		{
			attributes: map[string]any{
				"host.ip": "192.168.1.1",
			},
			key:         "host.ip",
			expected:    nil,
			expectedOk:  false,
			expectedErr: "\"host.ip\" has type \"Str\", expected type \"Slice\" instead",
		},
		{
			attributes: map[string]any{
				"host.ip": []any{},
			},
			key:         "host.ip",
			expected:    nil,
			expectedOk:  false,
			expectedErr: "\"host.ip\" is an empty slice, expected at least one item",
		},
		{
			attributes: map[string]any{
				"host.ip": []any{"192.168.1.1", true},
			},
			key:         "host.ip",
			expected:    nil,
			expectedOk:  false,
			expectedErr: "\"host.ip[1]\" has type \"Bool\", expected type \"Str\" instead",
		},
	}

	for _, tt := range tests {
		t.Run(tt.key+"/"+tt.expectedErr, func(t *testing.T) {
			res := testutils.NewResourceFromMap(t, tt.attributes)
			actual, ok, err := strSliceField(res.Attributes(), tt.key)
			assert.Equal(t, tt.expected, actual)
			assert.Equal(t, tt.expectedOk, ok)
			if tt.expectedErr != "" {
				assert.EqualError(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsIPv4(t *testing.T) {
	// Test cases come from https://stackoverflow.com/a/48519490
	tests := []struct {
		ip     string
		isIPv4 bool
	}{
		{ip: "192.168.0.1", isIPv4: true},
		{ip: "192.168.0.1:80", isIPv4: true},
		{ip: "::FFFF:C0A8:1", isIPv4: false},
		{ip: "::FFFF:C0A8:0001", isIPv4: false},
		{ip: "0000:0000:0000:0000:0000:FFFF:C0A8:1", isIPv4: false},
		{ip: "::FFFF:C0A8:1%1", isIPv4: false},
		{ip: "::FFFF:192.168.0.1", isIPv4: false},
		{ip: "[::FFFF:C0A8:1]:80", isIPv4: false},
		{ip: "[::FFFF:C0A8:1%1]:80", isIPv4: false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			assert.Equal(t, tt.isIPv4, isIPv4(tt.ip))
		})
	}
}

func TestIEEERAToGolangFormat(t *testing.T) {
	tests := []struct {
		ieeeRA       string
		golangFormat string
	}{
		{
			ieeeRA:       "AB-01-00-00-00-00-00-00",
			golangFormat: "ab:01:00:00:00:00:00:00",
		},
		{
			ieeeRA:       "AB-CD-EF-00-00-00",
			golangFormat: "ab:cd:ef:00:00:00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.ieeeRA, func(t *testing.T) {
			assert.Equal(t, tt.golangFormat, ieeeRAtoGolangFormat(tt.ieeeRA))
		})
	}
}

func TestUpdate(t *testing.T) {
	hostInfo := []struct {
		hostname        string
		attributes      map[string]any
		metric          *pmetric.Metric
		expectedChanged bool
		expectedErrs    []string
	}{
		{
			hostname: "host-1-hostid",
			attributes: map[string]any{
				string(conventions.CloudProviderKey):         conventions.CloudProviderAWS.Value.AsString(),
				string(conventions.CloudRegionKey):           "us-east-1",
				string(conventions.CloudAvailabilityZoneKey): "us-east-1c",
				string(conventions.HostIDKey):                "host-1-hostid",
				string(conventions.HostNameKey):              "host-1-hostname",
				string(conventions.OSDescriptionKey):         "Fedora Linux",
				string(conventions.OSTypeKey):                conventions.OSTypeLinux.Value.AsString(),
				string(conventions.HostArchKey):              conventions.HostArchAMD64.Value.AsString(),
				attributeKernelName:                          "GNU/Linux",
				attributeKernelRelease:                       "5.19.0-43-generic",
				attributeKernelVersion:                       "#44~22.04.1-Ubuntu SMP PREEMPT_DYNAMIC Mon May 22 13:39:36 UTC 2",
				attributeHostCPUVendorID:                     "GenuineIntel",
				attributeHostCPUFamily:                       6,
				attributeHostCPUModelID:                      10,
				attributeHostCPUModelName:                    "11th Gen Intel(R) Core(TM) i7-1185G7 @ 3.00GHz",
				attributeHostCPUStepping:                     1,
				attributeHostCPUCacheL2Size:                  12288000,
				attributeHostIP:                              []any{"192.168.1.140", "fe80::abc2:4a28:737a:609e"},
				attributeHostMAC:                             []any{"AC-DE-48-23-45-67", "AC-DE-48-23-45-67-01-9F"},
				"datadog.host.tag.foo":                       "bar",
				string(conventions.DeploymentEnvironmentKey): "prod",
			},
			metric:          BuildMetric[int64](metricSystemCPUPhysicalCount, 32),
			expectedChanged: true,
		},
		{
			// Same as #1, but missing some attributes
			hostname: "host-1-hostid",
			attributes: map[string]any{
				string(conventions.CloudProviderKey):         conventions.CloudProviderAWS.Value.AsString(),
				string(conventions.CloudRegionKey):           "us-east-1",
				string(conventions.CloudAvailabilityZoneKey): "us-east-1c",
				string(conventions.HostIDKey):                "host-1-hostid",
				string(conventions.HostNameKey):              "host-1-hostname",
				string(conventions.OSDescriptionKey):         "Fedora Linux",
				"datadog.host.tag.foo":                       "bar",
				string(conventions.DeploymentEnvironmentKey): "prod",
			},
			metric:          BuildMetric[float64](metricSystemCPUFrequency, 400_000_005.5),
			expectedChanged: false,
		},
		{
			// Same as #1 but wrong type and an update
			hostname: "host-1-hostid",
			attributes: map[string]any{
				string(conventions.CloudProviderKey):         conventions.CloudProviderAWS.Value.AsString(),
				string(conventions.CloudRegionKey):           "us-east-1",
				string(conventions.CloudAvailabilityZoneKey): "us-east-1c",
				string(conventions.HostIDKey):                "host-1-hostid",
				string(conventions.HostNameKey):              "host-1-hostname",
				string(conventions.OSDescriptionKey):         true, // wrong type
				string(conventions.HostArchKey):              conventions.HostArchAMD64.Value.AsString(),
				attributeKernelName:                          "GNU/Linux",
				attributeKernelRelease:                       "5.19.0-43-generic",
				attributeKernelVersion:                       "#82~18.04.1-Ubuntu SMP Fri Apr 16 15:10:02 UTC 2021", // changed
				"datadog.host.tag.foo":                       "baz",                                                 // changed
				string(conventions.DeploymentEnvironmentKey): "prod",
			},
			expectedChanged: true,
			expectedErrs:    []string{"\"os.description\" has type \"Bool\", expected type \"Str\" instead"},
		},
		{
			// Same as #1 but wrong type in two places and no update
			hostname: "host-1-hostid",
			attributes: map[string]any{
				string(conventions.CloudProviderKey):         conventions.CloudProviderAWS.Value.AsString(),
				string(conventions.CloudRegionKey):           "us-east-1",
				string(conventions.CloudAvailabilityZoneKey): "us-east-1c",
				string(conventions.HostIDKey):                "host-1-hostid",
				string(conventions.HostNameKey):              "host-1-hostname",
				string(conventions.OSDescriptionKey):         true, // wrong type
				string(conventions.HostArchKey):              conventions.HostArchAMD64.Value.AsString(),
				attributeKernelName:                          false, // wrong type
				attributeKernelRelease:                       "5.19.0-43-generic",
				"datadog.host.tag.foo":                       "baz",
				string(conventions.DeploymentEnvironmentKey): "prod",
			},
			expectedChanged: false,
			expectedErrs: []string{
				"\"os.description\" has type \"Bool\", expected type \"Str\" instead",
				"\"os.kernel.name\" has type \"Bool\", expected type \"Str\" instead",
			},
		},
		{
			// Different host, partial information, on Azure
			hostname: "host-2-hostid",
			attributes: map[string]any{
				string(conventions.CloudProviderKey): conventions.CloudProviderAzure.Value.AsString(),
				string(conventions.HostIDKey):        "host-2-hostid",
				string(conventions.HostNameKey):      "host-2-hostname",
				string(conventions.HostArchKey):      conventions.HostArchARM64.Value.AsString(),
				"deployment.environment.name":        "staging",
				"datadog.host.aliases":               []any{"host-2-hostid-alias-1", "host-2-hostid-alias-2"},
			},
			expectedChanged: true,
		},
		{
			// Same host, new aliases
			hostname: "host-2-hostid",
			attributes: map[string]any{
				string(conventions.CloudProviderKey): conventions.CloudProviderAzure.Value.AsString(),
				string(conventions.HostIDKey):        "host-2-hostid",
				string(conventions.HostNameKey):      "host-2-hostname",
				string(conventions.HostArchKey):      conventions.HostArchARM64.Value.AsString(),
				"deployment.environment.name":        "staging",
				"datadog.host.aliases":               []any{"host-2-hostid-alias-1", "host-2-hostid-alias-2", "host-2-hostid-alias-3"},
			},
			expectedChanged: true,
		},
	}

	hostMap := New()
	for _, info := range hostInfo {
		changed, _, err := hostMap.Update(info.hostname, testutils.NewResourceFromMap(t, info.attributes))
		assert.Equal(t, info.expectedChanged, changed)
		if len(info.expectedErrs) > 0 {
			errStrings := strings.Split(err.Error(), "\n")
			assert.ElementsMatch(t, info.expectedErrs, errStrings)
		} else {
			assert.NoError(t, err)
		}
		if info.metric != nil {
			hostMap.UpdateFromMetric(info.hostname, *info.metric)
		}
	}

	hosts := hostMap.Flush()
	assert.Len(t, hosts, 2)

	if assert.Contains(t, hosts, "host-1-hostid") {
		md := hosts["host-1-hostid"]
		assert.Equal(t, "host-1-hostid", md.InternalHostname)
		assert.Equal(t, "otelcol-contrib", md.Flavor)
		assert.Equal(t, &payload.Meta{
			InstanceID:  "host-1-hostid",
			EC2Hostname: "host-1-hostname",
			Hostname:    "host-1-hostid",
		}, md.Meta)
		assert.ElementsMatch(t, md.Tags.OTel, []string{"cloud_provider:aws", "region:us-east-1", "zone:us-east-1c", "foo:baz", "env:prod"})
		assert.Equal(t, map[string]any{
			"hostname":                    "host-1-hostid",
			fieldPlatformOS:               "Fedora Linux",
			fieldPlatformProcessor:        "amd64",
			fieldPlatformMachine:          "amd64",
			fieldPlatformHardwarePlatform: "amd64",
			fieldPlatformGOOS:             "linux",
			fieldPlatformGOOARCH:          "amd64",
			fieldPlatformKernelName:       "GNU/Linux",
			fieldPlatformKernelRelease:    "5.19.0-43-generic",
			fieldPlatformKernelVersion:    "#82~18.04.1-Ubuntu SMP Fri Apr 16 15:10:02 UTC 2021",
		}, md.Payload.Gohai.Gohai.Platform)
		assert.Equal(t, map[string]any{
			fieldCPUCacheSize: "12288000",
			fieldCPUFamily:    "6",
			fieldCPUModel:     "10",
			fieldCPUModelName: "11th Gen Intel(R) Core(TM) i7-1185G7 @ 3.00GHz",
			fieldCPUStepping:  "1",
			fieldCPUVendorID:  "GenuineIntel",
			fieldCPUCores:     "32",
			fieldCPUMHz:       "400.0000055",
		}, md.Payload.Gohai.Gohai.CPU)
		assert.Equal(t, map[string]any{
			fieldNetworkIPAddressIPv4: "192.168.1.140",
			fieldNetworkIPAddressIPv6: "fe80::abc2:4a28:737a:609e",
			fieldNetworkMACAddress:    "ac:de:48:23:45:67",
		}, md.Payload.Gohai.Gohai.Network)
		assert.Empty(t, md.Payload.Gohai.Gohai.FileSystem)
		assert.Empty(t, md.Payload.Gohai.Gohai.Memory)
	}

	if assert.Contains(t, hosts, "host-2-hostid") {
		md := hosts["host-2-hostid"]
		assert.Equal(t, "host-2-hostid", md.InternalHostname)
		assert.Equal(t, "otelcol-contrib", md.Flavor)
		assert.Equal(t, &payload.Meta{
			Hostname:    "host-2-hostid",
			HostAliases: []string{"host-2-hostid-alias-1", "host-2-hostid-alias-2", "host-2-hostid-alias-3"},
		}, md.Meta)
		assert.ElementsMatch(t, md.Tags.OTel, []string{"cloud_provider:azure", "env:staging"})
		assert.Equal(t, map[string]any{
			"hostname":                    "host-2-hostid",
			fieldPlatformProcessor:        "arm64",
			fieldPlatformMachine:          "arm64",
			fieldPlatformHardwarePlatform: "arm64",
			fieldPlatformGOOARCH:          "arm64",
		}, md.Platform())
		assert.Empty(t, md.Payload.Gohai.Gohai.CPU)
		assert.Empty(t, md.Payload.Gohai.Gohai.Network)
		assert.Empty(t, md.Payload.Gohai.Gohai.FileSystem)
		assert.Empty(t, md.Payload.Gohai.Gohai.Memory)
	}

	assert.Empty(t, hostMap.Flush(), "returned map must be empty after double flush")
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostinfo

import (
	"regexp"
	"testing"

	"github.com/shirou/gopsutil/v4/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gohai/platform"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

func TestFormatAIXPlatformVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"7300-02-02-2419", "7.3 TL2"},
		{"7200-01-00-1842", "7.2 TL1"},
		{"7300-00-00-0000", "7.3 TL0"},
		{"7300-10-05-2501", "7.3 TL10"},
		// not enough parts
		{"7300-02", ""},
		// Version.Release too short
		{"7-02-02-2419", ""},
		// empty string
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			aixVersion, ok := platform.ParseAIXVersion(tt.input)
			if tt.expected == "" {
				assert.False(t, ok)
			} else {
				assert.True(t, ok)
				assert.Equal(t, tt.expected, aixVersion.PlatformVersion())
			}
		})
	}
}

func TestGetHostInfoAIX(t *testing.T) {
	defer cache.Cache.Delete(hostInfoCacheKey)

	info := GetInformation()
	require.NotNil(t, info)

	// PlatformVersion must be in "V.R TLN" format, not the raw oslevel output.
	assert.Regexp(t, regexp.MustCompile(`^\d+\.\d+ TL\d+$`), info.PlatformVersion)

	// All other fields must match gopsutil directly.
	raw, err := host.Info()
	require.NoError(t, err)
	assert.Equal(t, raw.Hostname, info.Hostname)
	assert.Equal(t, raw.OS, info.OS)
	assert.Equal(t, raw.Platform, info.Platform)
	assert.Equal(t, raw.KernelVersion, info.KernelVersion)
	assert.Equal(t, raw.HostID, info.HostID)
}

func TestGetHostInfoAIXCache(t *testing.T) {
	defer cache.Cache.Delete(hostInfoCacheKey)

	fakeInfo := &host.InfoStat{HostID: "test data"}
	cache.Cache.Set(hostInfoCacheKey, fakeInfo, cache.NoExpiration)

	assert.Equal(t, fakeInfo, GetInformation())
}

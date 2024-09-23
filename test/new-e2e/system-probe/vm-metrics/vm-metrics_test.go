// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !windows

package main

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
	"github.com/digitalocean/go-libvirt"
	"github.com/stretchr/testify/require"
)

var memStats = map[libvirt.DomainMemoryStatTags]uint64{
	libvirt.DomainMemoryStatSwapIn:        10,
	libvirt.DomainMemoryStatSwapOut:       20,
	libvirt.DomainMemoryStatMajorFault:    30,
	libvirt.DomainMemoryStatAvailable:     40,
	libvirt.DomainMemoryStatActualBalloon: 50,
	libvirt.DomainMemoryStatRss:           60,
	libvirt.DomainMemoryStatUnused:        70,
	libvirt.DomainMemoryStatUsable:        80,
}

var nameToTag = map[string]libvirt.DomainMemoryStatTags{
	"swap_in_bytes":               libvirt.DomainMemoryStatSwapIn,
	"swap_out_bytes":              libvirt.DomainMemoryStatSwapOut,
	"major_pagefault":             libvirt.DomainMemoryStatMajorFault,
	"memory_available_bytes":      libvirt.DomainMemoryStatAvailable,
	"memory_actual_balloon_bytes": libvirt.DomainMemoryStatActualBalloon,
	"memory_rss_bytes":            libvirt.DomainMemoryStatRss,
	"memory_usable_bytes":         libvirt.DomainMemoryStatUsable,
	"memory_unused_bytes":         libvirt.DomainMemoryStatUnused,
}

func TestParseOSInformation(t *testing.T) {
	cases := map[string]string{
		"x86_64-fedora_37-distro_x86_64-no_usm-ddvm-4-12288":        "fedora_37",
		"x86_64-fedora_38-distro_x86_64-no_usm-ddvm-4-12288":        "fedora_38",
		"x86_64-amazon_4.14-distro_x86_64-no_usm-ddvm-4-12288":      "amazon_4.14",
		"x86_64-amazon_5.10-distro_x86_64-no_usm-ddvm-4-12288":      "amazon_5.10",
		"x86_64-amazon_5.4-distro_x86_64-no_usm-ddvm-4-12288":       "amazon_5.4",
		"x86_64-amazon_2023-distro_x86_64-no_usm-ddvm-4-12288":      "amazon_2023",
		"x86_64-centos_7.9-distro_x86_64-no_usm-ddvm-4-12288":       "centos_7.9",
		"x86_64-centos_8-distro_x86_64-no_usm-ddvm-4-12288":         "centos_8",
		"x86_64-ubuntu_24.04-all_tests-distro_x86_64-ddvm-4-12288":  "ubuntu_24.04",
		"arm64-ubuntu_23.10-distro_arm64-no_usm-ddvm-4-12288":       "ubuntu_23.10",
		"arm64-ubuntu_22.04-distro_arm64-no_usm-ddvm-4-12288":       "ubuntu_22.04",
		"arm64-ubuntu_20.04-distro_arm64-no_usm-ddvm-4-12288":       "ubuntu_20.04",
		"arm64-ubuntu_18.04-distro_arm64-no_usm-ddvm-4-12288":       "ubuntu_18.04",
		"x86_64-ubuntu_16.04-distro_x86_64-no_usm-ddvm-4-12288":     "ubuntu_16.04",
		"x86_64-debian_9-distro_x86_64-no_usm-ddvm-4-12288":         "debian_9",
		"x86_64-debian_10-only_usm-distro_x86_64-ddvm-4-12288":      "debian_10",
		"x86_64-debian_11-only_usm-distro_x86_64-ddvm-4-12288":      "debian_11",
		"x86_64-debian_12-only_usm-distro_x86_64-ddvm-4-12288":      "debian_12",
		"x86_64-suse_12.5-all_tests-distro_x86_64-ddvm-4-12288":     "suse_12.5",
		"x86_64-opensuse_15.5-all_tests-distro_x86_64-ddvm-4-12288": "opensuse_15.5",
		"x86_64-opensuse_15.3-all_tests-distro_x86_64-ddvm-4-12288": "opensuse_15.3",
		"x86_64-rocky_9.3-all_tests-distro_x86_64-ddvm-4-12288":     "rocky_9.3",
		"x86_64-rocky_8.5-all_tests-distro_x86_64-ddvm-4-12288":     "rocky_8.5",
		"x86_64-oracle_9.3-all_tests-distro_x86_64-ddvm-4-12288":    "oracle_9.3",
		"x86_64-oracle_8.9-all_tests-distro_x86_64-ddvm-4-12288":    "oracle_8.9",
	}

	for id, os := range cases {
		osID := parseOSInformation(id)
		require.Equal(t, osID, os)
	}
}

type libvirtMock struct{}

func (l *libvirtMock) ConnectListAllDomains(_ int32, _ libvirt.ConnectListAllDomainsFlags) ([]libvirt.Domain, uint32, error) {
	return []libvirt.Domain{
		{Name: "x86_64-debian_12-only_usm-distro_x86_64-ddvm-4-12288"},
		{Name: "x86_64-ubuntu_16.04-distro_x86_64-no_usm-ddvm-4-12288"},
	}, 0, nil
}

func (l *libvirtMock) DomainMemoryStats(_ libvirt.Domain, _ uint32, _ uint32) ([]libvirt.DomainMemoryStat, error) {
	var stats []libvirt.DomainMemoryStat
	for tag, val := range memStats {
		stats = append(stats, libvirt.DomainMemoryStat{
			Tag: int32(tag),
			Val: val,
		})
	}
	return stats, nil
}

func bytesToKb(bytes uint64) uint64 {
	return bytes / 1024
}

func TestLibvirtCollectMetrics(t *testing.T) {
	lexporter := newLibvirtExporter(&libvirtMock{}, &teststatsd.Client{})

	domainMetrics, err := lexporter.collect()
	require.NoError(t, err)

	for _, dm := range domainMetrics {
		for _, m := range dm.metrics {
			tag, ok := nameToTag[m.name]
			require.True(t, ok)

			if tag == libvirt.DomainMemoryStatMajorFault {
				require.Equal(t, memStats[tag], m.value)
			} else {
				require.Equal(t, memStats[tag], bytesToKb(m.value))
			}
		}
	}
}
func TestLibvirtSubmitMetrics(t *testing.T) {
	lexporter := newLibvirtExporter(&libvirtMock{}, &teststatsd.Client{})

	domainMetrics, err := lexporter.collect()
	require.NoError(t, err)

	err = lexporter.submit(domainMetrics)
	require.NoError(t, err)

	for name, summary := range lexporter.statsdClient.(*teststatsd.Client).GetGaugeSummaries() {
		statName := strings.TrimPrefix(name, kmtMicroVmsPrefix)
		expectedVal := memStats[nameToTag[statName]]
		if statName != "major_pagefault" {
			expectedVal *= 1024
		}

		for _, call := range summary.Calls {
			require.Equal(t, call.Value, float64(expectedVal))
		}
	}
}

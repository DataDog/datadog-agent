// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package disk_test

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk/disk"
	"github.com/stretchr/testify/assert"
)

func setupPlatformMocks() {
	disk.LsblkCommand = func() (string, error) {
		return LsblkData, nil
	}
	disk.BlkidCacheCommand = func(_blkidCacheFile string) (string, error) {
		return blkidCacheData, nil
	}
	disk.BlkidCommand = func() (string, error) {
		return blkidData, nil
	}
}

func TestGivenADiskCheckWithTagByLabelConfiguredTrue_WhenCheckRunsAndBlkidReturnsError_ThenBlkidLabelsAreNotReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	disk.BlkidCommand = func() (string, error) {
		return "", errors.New("error calling blkid")
	}
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
tag_by_label: true
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
}

func TestGivenADiskCheckWithUseLsblkConfiguredTrue_WhenCheckRuns_ThenLsblkLabelsAreReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
use_lsblk: true
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
}

func TestGivenADiskCheckWithTagByLabelConfiguredTrueAndUseLsblk_WhenCheckRuns_ThenLsblkLabelsAreNotReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
tag_by_label: true
use_lsblk: true
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
}

func TestGivenADiskCheckWithTagByLabelConfiguredFalseAndUseLsblk_WhenCheckRuns_ThenLsblkLabelsAreNotReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
tag_by_label: false
use_lsblk: true
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
}

func TestGivenADiskCheckWithUseLsblkConfiguredTrue_WhenLsblkReturnsError_ThenLsblkLabelsAreNotReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	disk.LsblkCommand = func() (string, error) {
		return "", errors.New("error calling lsblk")
	}
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
use_lsblk: true
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
}

func TestGivenADiskCheckWithBlkidCacheFileConfigured_WhenCheckRuns_ThenBlkidCacheFileLabelsAreReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	var actualBlkidCacheFile string
	disk.BlkidCacheCommand = func(blkidCacheFile string) (string, error) {
		actualBlkidCacheFile = blkidCacheFile
		return blkidCacheData, nil
	}
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
blkid_cache_file: /run/blkid/blkid.tab
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	assert.Equal(t, "/run/blkid/blkid.tab", actualBlkidCacheFile)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda2", "device_name:sda2", "label:BACKUP", "device_label:BACKUP"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda2", "device_name:sda2", "label:BACKUP", "device_label:BACKUP"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda2", "device_name:sda2", "label:BACKUP", "device_label:BACKUP"})
}

func TestGivenADiskCheckWithBlkidCacheFileConfigured_WhenBlkidCacheReturnsError_ThenBlkidCacheLabelsAreNotReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	var actualBlkidCacheFile string
	disk.BlkidCacheCommand = func(blkidCacheFile string) (string, error) {
		actualBlkidCacheFile = blkidCacheFile
		return "", errors.New("error calling blkid cache")
	}
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
blkid_cache_file: /run/blkid/blkid.tab
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	assert.Equal(t, "/run/blkid/blkid.tab", actualBlkidCacheFile)
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:BACKUP", "device_label:BACKUP"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:BACKUP", "device_label:BACKUP"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:BACKUP", "device_label:BACKUP"})
}

func TestGivenADiskCheckWithBlkidCacheFileConfigured_WhenBlkidCacheHasWrongLines_ThenBlkidCacheLabelsAreNotReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	var actualBlkidCacheFile string
	disk.BlkidCacheCommand = func(blkidCacheFile string) (string, error) {
		actualBlkidCacheFile = blkidCacheFile
		return string(`
<device DEVNO="0x0801" LABEL="MYLABEL" UUID="1234-5678" TYPE="ext4">/dev/sda1</device>

<device DEVNO="0x0802" LABEL="BACKUP" UUID="8765-4321" TYPE="ext4">/dev/sda2</device
<device DEVNO="0x0811" LABEL="USB_DRIVE" UUID="abcd-efgh" TYPE="vfat">/dev/sdb1</device>
<device DEVNO="0x0812" LABEL="DATA_DISK" UUID="ijkl-mnop" TYPE="ntfs">/dev/sdb2</device>
`), nil
	}
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
blkid_cache_file: /run/blkid/blkid.tab
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	assert.Equal(t, "/run/blkid/blkid.tab", actualBlkidCacheFile)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:BACKUP", "device_label:BACKUP"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:BACKUP", "device_label:BACKUP"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:BACKUP", "device_label:BACKUP"})
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package disk

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	pdhtest "github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

func testGetDriveType(drive string) uintptr {
	return DRIVE_FIXED
}

func addDefaultQueryReturnValues() {
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(_Total)\\Disk Write Bytes/sec", 1.111)
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(C:)\\Disk Write Bytes/sec", 1.222)
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(HarddiskVolume1)\\Disk Write Bytes/sec", 1.333)

	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(_Total)\\Disk Writes/sec", 2.111)
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(C:)\\Disk Writes/sec", 2.222)
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(HarddiskVolume1)\\Disk Writes/sec", 2.333)

	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(_Total)\\Disk Read Bytes/sec", 3.111)
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(C:)\\Disk Read Bytes/sec", 3.222)
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(HarddiskVolume1)\\Disk Read Bytes/sec", 3.333)

	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(_Total)\\Disk Reads/sec", 4.111)
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(C:)\\Disk Reads/sec", 4.222)
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(HarddiskVolume1)\\Disk Reads/sec", 4.333)

	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(_Total)\\Current Disk Queue Length", 5.111)
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(C:)\\Current Disk Queue Length", 5.222)
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(HarddiskVolume1)\\Current Disk Queue Length", 5.333)
	return
}

func addDriveDReturnValues() {
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(Y:)\\Disk Write Bytes/sec", 1.444)
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(HarddiskVolume2)\\Disk Write Bytes/sec", 1.555)

	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(Y:)\\Disk Writes/sec", 2.444)
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(HarddiskVolume2)\\Disk Writes/sec", 2.555)

	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(Y:)\\Disk Read Bytes/sec", 3.444)
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(HarddiskVolume2)\\Disk Read Bytes/sec", 3.555)

	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(Y:)\\Disk Reads/sec", 4.444)
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(HarddiskVolume2)\\Disk Reads/sec", 4.555)

	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(Y:)\\Current Disk Queue Length", 5.444)
	pdhtest.SetQueryReturnValue("\\\\.\\LogicalDisk(HarddiskVolume2)\\Current Disk Queue Length", 5.555)
}

func TestIoCheckWindows(t *testing.T) {
	pfnGetDriveType = testGetDriveType
	pdhtest.SetupTesting("..\\testfiles\\counter_indexes_en-us.txt", "..\\testfiles\\allcounters_en-us.txt")

	addDefaultQueryReturnValues()

	ioCheck := new(IOCheck)
	mock := mocksender.NewMockSender(ioCheck.ID())
	ioCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	mock.On("Gauge", "system.io.wkb_s", 1.222/kB, "", []string{"device:C:"}).Return().Times(1)
	mock.On("Gauge", "system.io.wkb_s", 1.333/kB, "", []string{"device:HarddiskVolume1", "device_name:HarddiskVolume1"}).Return().Times(1)

	mock.On("Gauge", "system.io.w_s", 2.222, "", []string{"device:C:"}).Return().Times(1)
	mock.On("Gauge", "system.io.w_s", 2.333, "", []string{"device:HarddiskVolume1", "device_name:HarddiskVolume1"}).Return().Times(1)

	mock.On("Gauge", "system.io.rkb_s", 3.222/kB, "", []string{"device:C:"}).Return().Times(1)
	mock.On("Gauge", "system.io.rkb_s", 3.333/kB, "", []string{"device:HarddiskVolume1", "device_name:HarddiskVolume1"}).Return().Times(1)

	mock.On("Gauge", "system.io.r_s", 4.222, "", []string{"device:C:"}).Return().Times(1)
	mock.On("Gauge", "system.io.r_s", 4.333, "", []string{"device:HarddiskVolume1", "device_name:HarddiskVolume1"}).Return().Times(1)

	mock.On("Gauge", "system.io.avg_q_sz", 5.222, "", []string{"device:C:"}).Return().Times(1)
	mock.On("Gauge", "system.io.avg_q_sz", 5.333, "", []string{"device:HarddiskVolume1", "device_name:HarddiskVolume1"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)
	ioCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 10)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestIoCheckLowercaseDeviceTag(t *testing.T) {
	pfnGetDriveType = testGetDriveType
	pdhtest.SetupTesting("..\\testfiles\\counter_indexes_en-us.txt", "..\\testfiles\\allcounters_en-us.txt")

	addDefaultQueryReturnValues()

	ioCheck := new(IOCheck)
	rawInitConfigYaml := []byte(`
lowercase_device_tag: true
`)
	mock := mocksender.NewMockSender(ioCheck.ID())
	err := ioCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, rawInitConfigYaml, "test")
	require.NoError(t, err)

	mock.On("Gauge", "system.io.wkb_s", 1.222/kB, "", []string{"device:c:"}).Return().Times(1)
	mock.On("Gauge", "system.io.wkb_s", 1.333/kB, "", []string{"device:harddiskvolume1", "device_name:harddiskvolume1"}).Return().Times(1)

	mock.On("Gauge", "system.io.w_s", 2.222, "", []string{"device:c:"}).Return().Times(1)
	mock.On("Gauge", "system.io.w_s", 2.333, "", []string{"device:harddiskvolume1", "device_name:harddiskvolume1"}).Return().Times(1)

	mock.On("Gauge", "system.io.rkb_s", 3.222/kB, "", []string{"device:c:"}).Return().Times(1)
	mock.On("Gauge", "system.io.rkb_s", 3.333/kB, "", []string{"device:harddiskvolume1", "device_name:harddiskvolume1"}).Return().Times(1)

	mock.On("Gauge", "system.io.r_s", 4.222, "", []string{"device:c:"}).Return().Times(1)
	mock.On("Gauge", "system.io.r_s", 4.333, "", []string{"device:harddiskvolume1", "device_name:harddiskvolume1"}).Return().Times(1)

	mock.On("Gauge", "system.io.avg_q_sz", 5.222, "", []string{"device:c:"}).Return().Times(1)
	mock.On("Gauge", "system.io.avg_q_sz", 5.333, "", []string{"device:harddiskvolume1", "device_name:harddiskvolume1"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)
	ioCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 10)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestIoCheckInstanceAdded(t *testing.T) {
	pfnGetDriveType = testGetDriveType
	pdhtest.SetupTesting("..\\testfiles\\counter_indexes_en-us.txt", "..\\testfiles\\allcounters_en-us.txt")

	addDefaultQueryReturnValues()
	// add second set of returns for the second run
	addDefaultQueryReturnValues()
	// add new returns for the new instance
	addDriveDReturnValues()

	ioCheck := new(IOCheck)
	mock := mocksender.NewMockSender(ioCheck.ID())
	ioCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	pdhtest.AddCounterInstance("LogicalDisk", "Y:")
	pdhtest.AddCounterInstance("LogicalDisk", "HarddiskVolume2")

	mock.On("Gauge", "system.io.wkb_s", 1.222/kB, "", []string{"device:C:"}).Return().Times(2)
	mock.On("Gauge", "system.io.wkb_s", 1.333/kB, "", []string{"device:HarddiskVolume1", "device_name:HarddiskVolume1"}).Return().Times(2)

	mock.On("Gauge", "system.io.w_s", 2.222, "", []string{"device:C:"}).Return().Times(2)
	mock.On("Gauge", "system.io.w_s", 2.333, "", []string{"device:HarddiskVolume1", "device_name:HarddiskVolume1"}).Return().Times(2)

	mock.On("Gauge", "system.io.rkb_s", 3.222/kB, "", []string{"device:C:"}).Return().Times(2)
	mock.On("Gauge", "system.io.rkb_s", 3.333/kB, "", []string{"device:HarddiskVolume1", "device_name:HarddiskVolume1"}).Return().Times(2)

	mock.On("Gauge", "system.io.r_s", 4.222, "", []string{"device:C:"}).Return().Times(2)
	mock.On("Gauge", "system.io.r_s", 4.333, "", []string{"device:HarddiskVolume1", "device_name:HarddiskVolume1"}).Return().Times(2)

	mock.On("Gauge", "system.io.avg_q_sz", 5.222, "", []string{"device:C:"}).Return().Times(2)
	mock.On("Gauge", "system.io.avg_q_sz", 5.333, "", []string{"device:HarddiskVolume1", "device_name:HarddiskVolume1"}).Return().Times(2)

	// and the checks for the added instance
	mock.On("Gauge", "system.io.wkb_s", 1.444/kB, "", []string{"device:Y:"}).Return().Times(1)
	mock.On("Gauge", "system.io.wkb_s", 1.555/kB, "", []string{"device:HarddiskVolume2", "device_name:HarddiskVolume2"}).Return().Times(1)

	mock.On("Gauge", "system.io.w_s", 2.444, "", []string{"device:Y:"}).Return().Times(1)
	mock.On("Gauge", "system.io.w_s", 2.555, "", []string{"device:HarddiskVolume2", "device_name:HarddiskVolume2"}).Return().Times(1)

	mock.On("Gauge", "system.io.rkb_s", 3.444/kB, "", []string{"device:Y:"}).Return().Times(1)
	mock.On("Gauge", "system.io.rkb_s", 3.555/kB, "", []string{"device:HarddiskVolume2", "device_name:HarddiskVolume2"}).Return().Times(1)

	mock.On("Gauge", "system.io.r_s", 4.444, "", []string{"device:Y:"}).Return().Times(1)
	mock.On("Gauge", "system.io.r_s", 4.555, "", []string{"device:HarddiskVolume2", "device_name:HarddiskVolume2"}).Return().Times(1)

	mock.On("Gauge", "system.io.avg_q_sz", 5.444, "", []string{"device:Y:"}).Return().Times(1)
	mock.On("Gauge", "system.io.avg_q_sz", 5.555, "", []string{"device:HarddiskVolume2", "device_name:HarddiskVolume2"}).Return().Times(1)
	mock.On("Commit").Return().Times(2)
	ioCheck.Run()
	ioCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 30)
	mock.AssertNumberOfCalls(t, "Commit", 2)
}

func TestIoCheckInstanceRemoved(t *testing.T) {
	pfnGetDriveType = testGetDriveType
	pdhtest.SetupTesting("..\\testfiles\\counter_indexes_en-us.txt", "..\\testfiles\\allcounters_en-us.txt")
	pdhtest.AddCounterInstance("LogicalDisk", "Y:")
	pdhtest.AddCounterInstance("LogicalDisk", "HarddiskVolume2")

	addDefaultQueryReturnValues()
	// add second set of returns for the second run
	addDefaultQueryReturnValues()
	// add a third set of returns for the third run
	addDefaultQueryReturnValues()
	// add new returns for the new instance
	addDriveDReturnValues()

	ioCheck := new(IOCheck)
	mock := mocksender.NewMockSender(ioCheck.ID())
	ioCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	mock.On("Gauge", "system.io.wkb_s", 1.222/kB, "", []string{"device:C:"}).Return().Times(3)
	mock.On("Gauge", "system.io.wkb_s", 1.333/kB, "", []string{"device:HarddiskVolume1", "device_name:HarddiskVolume1"}).Return().Times(3)

	mock.On("Gauge", "system.io.w_s", 2.222, "", []string{"device:C:"}).Return().Times(3)
	mock.On("Gauge", "system.io.w_s", 2.333, "", []string{"device:HarddiskVolume1", "device_name:HarddiskVolume1"}).Return().Times(3)

	mock.On("Gauge", "system.io.rkb_s", 3.222/kB, "", []string{"device:C:"}).Return().Times(3)
	mock.On("Gauge", "system.io.rkb_s", 3.333/kB, "", []string{"device:HarddiskVolume1", "device_name:HarddiskVolume1"}).Return().Times(3)

	mock.On("Gauge", "system.io.r_s", 4.222, "", []string{"device:C:"}).Return().Times(3)
	mock.On("Gauge", "system.io.r_s", 4.333, "", []string{"device:HarddiskVolume1", "device_name:HarddiskVolume1"}).Return().Times(3)

	mock.On("Gauge", "system.io.avg_q_sz", 5.222, "", []string{"device:C:"}).Return().Times(3)
	mock.On("Gauge", "system.io.avg_q_sz", 5.333, "", []string{"device:HarddiskVolume1", "device_name:HarddiskVolume1"}).Return().Times(3)

	// and the checks for the added instance
	mock.On("Gauge", "system.io.wkb_s", 1.444/kB, "", []string{"device:Y:"}).Return().Times(1)
	mock.On("Gauge", "system.io.wkb_s", 1.555/kB, "", []string{"device:HarddiskVolume2", "device_name:HarddiskVolume2"}).Return().Times(1)

	mock.On("Gauge", "system.io.w_s", 2.444, "", []string{"device:Y:"}).Return().Times(1)
	mock.On("Gauge", "system.io.w_s", 2.555, "", []string{"device:HarddiskVolume2", "device_name:HarddiskVolume2"}).Return().Times(1)

	mock.On("Gauge", "system.io.rkb_s", 3.444/kB, "", []string{"device:Y:"}).Return().Times(1)
	mock.On("Gauge", "system.io.rkb_s", 3.555/kB, "", []string{"device:HarddiskVolume2", "device_name:HarddiskVolume2"}).Return().Times(1)

	mock.On("Gauge", "system.io.r_s", 4.444, "", []string{"device:Y:"}).Return().Times(1)
	mock.On("Gauge", "system.io.r_s", 4.555, "", []string{"device:HarddiskVolume2", "device_name:HarddiskVolume2"}).Return().Times(1)

	mock.On("Gauge", "system.io.avg_q_sz", 5.444, "", []string{"device:Y:"}).Return().Times(1)
	mock.On("Gauge", "system.io.avg_q_sz", 5.555, "", []string{"device:HarddiskVolume2", "device_name:HarddiskVolume2"}).Return().Times(1)
	mock.On("Commit").Return().Times(3)
	ioCheck.Run()
	pdhtest.RemoveCounterInstance("LogicalDisk", "Y:")
	pdhtest.RemoveCounterInstance("LogicalDisk", "HarddiskVolume2")

	ioCheck.Run()
	ioCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 40)
	mock.AssertNumberOfCalls(t, "Commit", 3)
}

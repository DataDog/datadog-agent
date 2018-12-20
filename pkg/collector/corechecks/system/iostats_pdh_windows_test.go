// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build windows

package system

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	pdhtest "github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

func TestIoCheckWindows(t *testing.T) {

	pdhtest.SetupTesting("testfiles\\counter_indexes_en-us.txt", "testfiles\\allcounters_en-us.txt")

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

	ioCheck := new(IOCheck)
	ioCheck.Configure(nil, nil)

	mock := mocksender.NewMockSender(ioCheck.ID())

	mock.On("Gauge", "system.io.wkb_s", 1.222/1024, "", []string{"device:C:"}).Return().Times(1)
	mock.On("Gauge", "system.io.wkb_s", 1.333/1024, "", []string{"device:HarddiskVolume1"}).Return().Times(1)

	mock.On("Gauge", "system.io.w_s", 2.222, "", []string{"device:C:"}).Return().Times(1)
	mock.On("Gauge", "system.io.w_s", 2.333, "", []string{"device:HarddiskVolume1"}).Return().Times(1)

	mock.On("Gauge", "system.io.rkb_s", 3.222/1024, "", []string{"device:C:"}).Return().Times(1)
	mock.On("Gauge", "system.io.rkb_s", 3.333/1024, "", []string{"device:HarddiskVolume1"}).Return().Times(1)

	mock.On("Gauge", "system.io.r_s", 4.222, "", []string{"device:C:"}).Return().Times(1)
	mock.On("Gauge", "system.io.r_s", 4.333, "", []string{"device:HarddiskVolume1"}).Return().Times(1)

	mock.On("Gauge", "system.io.avg_q_sz", 5.222, "", []string{"device:C:"}).Return().Times(1)
	mock.On("Gauge", "system.io.avg_q_sz", 5.333, "", []string{"device:HarddiskVolume1"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)
	ioCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 10)
	mock.AssertNumberOfCalls(t, "Commit", 1)

}

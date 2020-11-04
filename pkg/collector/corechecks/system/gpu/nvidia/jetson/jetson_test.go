// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build jetson

package nvidia

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

const (
	tx1Sample      = "RAM 1179/3983MB (lfb 120x4MB) IRAM 0/252kB(lfb 252kB) CPU [1%@102,4%@102,0%@102,0%@102] EMC_FREQ 7%@408 GR3D_FREQ 0%@76 APE 25 AO@42.5C CPU@37.5C GPU@39C PLL@37C Tdiode@42.75C PMIC@100C Tboard@42C thermal@38.5C VDD_IN 2532/2698 VDD_CPU 76/178 VDD_GPU 19/19"
	tx2Sample      = "RAM 2344/7852MB (lfb 1154x4MB) SWAP 0/3926MB (cached 0MB) CPU [1%@345,off,off,1%@345,0%@345,2%@345] EMC_FREQ 4%@1600 GR3D_FREQ 0%@624 APE 150 PLL@37.5C MCPU@37.5C PMIC@100C Tboard@32C GPU@35C BCPU@37.5C thermal@36.5C Tdiode@34.25C VDD_SYS_GPU 152/152 VDD_SYS_SOC 687/687 VDD_4V0_WIFI 0/0 VDD_IN 3056/3056 VDD_SYS_CPU 152/152 VDD_SYS_DDR 883/883"
	nanoSample     = "RAM 534/3964MB (lfb 98x4MB) SWAP 5/1982MB (cached 1MB) IRAM 0/252kB(lfb 252kB) CPU [16%@204,9%@204,0%@204,0%@204] EMC_FREQ 0%@204 GR3D_FREQ 0%@76 APE 25 PLL@34C CPU@36.5C PMIC@100C GPU@36C AO@39.5C thermal@36.25C POM_5V_IN 1022/1022 POM_5V_GPU 0/0 POM_5V_CPU 204/204"
	agXSample      = "RAM 721/31927MB (lfb 7291x4MB) SWAP 0/15963MB (cached 0MB) CPU [2%@1190,0%@1190,0%@1190,0%@1190,off,off,off,off] EMC_FREQ 0%@665 GR3D_FREQ 0%@318 APE 150 MTS fg 0% bg 0% AO@37.5C GPU@37.5C Tdiode@40C PMIC@100C AUX@36C CPU@37.5C thermal@36.9C Tboard@37C GPU 0/0 CPU 311/311 SOC 932/932 CV 0/0 VDDRQ 621/621 SYS5V 1482/1482"
	xavierNxSample = "RAM 4412/7772MB (lfb 237x4MB) SWAP 139/3886MB (cached 2MB) CPU [9%@1190,6%@1190,6%@1190,5%@1190,4%@1190,8%@1267] EMC_FREQ 10%@1600 GR3D_FREQ 62%@306 APE 150 MTS fg 0% bg 0% AO@41.5C GPU@43C PMIC@100C AUX@41.5C CPU@43.5C thermal@42.55C VDD_IN 4067/4067 VDD_CPU_GPU_CV 738/738 VDD_SOC 1353/1353"
)

func TestNano(t *testing.T) {

	tegraCheck := new(JetsonCheck)
	tegraCheck.Configure(nil, nil, "test")

	assert.Equal(t, tegraCheck.tegraStatsPath, "/usr/bin/tegrastats")

	mock := mocksender.NewMockSender(tegraCheck.ID())
	mock.On("Gauge", "nvidia.jetson.mem.used", 534.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.total", 3964.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.n_lfb", 98.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.lfb", 4.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.used", 5.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.total", 1982.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.cached", 1.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.iram.used", 0.0*kb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.iram.total", 252.0*kb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.iram.lfb", 252.0*kb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.emc.usage", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.emc.freq", 204.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.gpu.usage", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.gpu.freq", 76.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 16.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 204.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 9.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 204.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 204.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:3"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 204.0, "", []string{"cpu:3"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.inactive_count", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.total_count", 4.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 39.5, "", []string{"zone:AO"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 36.5, "", []string{"zone:CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 36.0, "", []string{"zone:GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 34.0, "", []string{"zone:PLL"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 36.25, "", []string{"zone:thermal"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 100.0, "", []string{"zone:PMIC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 1022.0, "", []string{"probe:POM_5V_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 1022.0, "", []string{"probe:POM_5V_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 0.0, "", []string{"probe:POM_5V_GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 0.0, "", []string{"probe:POM_5V_GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 204.0, "", []string{"probe:POM_5V_CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 204.0, "", []string{"probe:POM_5V_CPU"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := tegraCheck.processTegraStatsOutput(nanoSample)
	assert.Equal(t, err, nil)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 36)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestTX1(t *testing.T) {

	tegraCheck := new(JetsonCheck)
	tegraCheck.Configure(nil, nil, "test")

	assert.Equal(t, tegraCheck.tegraStatsPath, "/usr/bin/tegrastats")

	mock := mocksender.NewMockSender(tegraCheck.ID())
	mock.On("Gauge", "nvidia.jetson.mem.used", 1179.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.total", 3983.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.n_lfb", 120.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.lfb", 4.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.iram.used", 0.0*kb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.iram.total", 252.0*kb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.iram.lfb", 252.0*kb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.emc.usage", 7.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.emc.freq", 408.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.gpu.usage", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.gpu.freq", 76.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 1.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 102.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 4.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 102.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 102.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:3"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 102.0, "", []string{"cpu:3"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.inactive_count", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.total_count", 4.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 42.5, "", []string{"zone:AO"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 37.5, "", []string{"zone:CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 39.0, "", []string{"zone:GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 37.0, "", []string{"zone:PLL"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 42.75, "", []string{"zone:Tdiode"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 100.0, "", []string{"zone:PMIC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 42.0, "", []string{"zone:Tboard"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 38.5, "", []string{"zone:thermal"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 2532.0, "", []string{"probe:VDD_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 2698.0, "", []string{"probe:VDD_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 76.0, "", []string{"probe:VDD_CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 178.0, "", []string{"probe:VDD_CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 19.0, "", []string{"probe:VDD_GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 19.0, "", []string{"probe:VDD_GPU"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := tegraCheck.processTegraStatsOutput(tx1Sample)
	assert.Equal(t, err, nil)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 35)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestTX2(t *testing.T) {

	tegraCheck := new(JetsonCheck)
	tegraCheck.Configure(nil, nil, "test")

	assert.Equal(t, tegraCheck.tegraStatsPath, "/usr/bin/tegrastats")

	mock := mocksender.NewMockSender(tegraCheck.ID())
	mock.On("Gauge", "nvidia.jetson.mem.used", 2344.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.total", 7852.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.n_lfb", 1154.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.lfb", 4.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.used", 0.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.total", 3926.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.cached", 0.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 1.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 345.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 0.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 0.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 1.0, "", []string{"cpu:3"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 345.0, "", []string{"cpu:3"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:4"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 345.0, "", []string{"cpu:4"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 2.0, "", []string{"cpu:5"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 345.0, "", []string{"cpu:5"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.inactive_count", 2.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.total_count", 6.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.emc.usage", 4.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.emc.freq", 1600.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.gpu.usage", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.gpu.freq", 624.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 37.5, "", []string{"zone:MCPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 37.5, "", []string{"zone:BCPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 35.0, "", []string{"zone:GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 37.5, "", []string{"zone:PLL"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 34.25, "", []string{"zone:Tdiode"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 100.0, "", []string{"zone:PMIC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 32.0, "", []string{"zone:Tboard"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 36.5, "", []string{"zone:thermal"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 152.0, "", []string{"probe:VDD_SYS_GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 152.0, "", []string{"probe:VDD_SYS_GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 687.0, "", []string{"probe:VDD_SYS_SOC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 687.0, "", []string{"probe:VDD_SYS_SOC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 0.0, "", []string{"probe:VDD_4V0_WIFI"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 0.0, "", []string{"probe:VDD_4V0_WIFI"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 3056.0, "", []string{"probe:VDD_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 3056.0, "", []string{"probe:VDD_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 152.0, "", []string{"probe:VDD_SYS_CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 152.0, "", []string{"probe:VDD_SYS_CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 883.0, "", []string{"probe:VDD_SYS_DDR"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 883.0, "", []string{"probe:VDD_SYS_DDR"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := tegraCheck.processTegraStatsOutput(tx2Sample)
	assert.Equal(t, err, nil)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 45)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestAgxXavier(t *testing.T) {

	tegraCheck := new(JetsonCheck)
	tegraCheck.Configure(nil, nil, "test")

	assert.Equal(t, tegraCheck.tegraStatsPath, "/usr/bin/tegrastats")
	mock := mocksender.NewMockSender(tegraCheck.ID())
	mock.On("Gauge", "nvidia.jetson.mem.used", 721.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.total", 31927.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.n_lfb", 7291.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.lfb", 4.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.used", 0.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.total", 15963.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.cached", 0.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 2.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 1190.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 1190.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 1190.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:3"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 1190.0, "", []string{"cpu:3"}).Return().Times(1)
	// Off cpus report 0 usage and 0 frequency
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:4"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 0.0, "", []string{"cpu:4"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:5"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 0.0, "", []string{"cpu:5"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:6"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 0.0, "", []string{"cpu:6"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:7"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 0.0, "", []string{"cpu:7"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.gpu.usage", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.gpu.freq", 318.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.emc.usage", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.emc.freq", 665.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.inactive_count", 4.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.total_count", 8.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 37.5, "", []string{"zone:AO"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 37.5, "", []string{"zone:CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 37.5, "", []string{"zone:GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 36.0, "", []string{"zone:AUX"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 40.0, "", []string{"zone:Tdiode"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 100.0, "", []string{"zone:PMIC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 37.0, "", []string{"zone:Tboard"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 36.9, "", []string{"zone:thermal"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 932.0, "", []string{"probe:SOC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 932.0, "", []string{"probe:SOC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 311.0, "", []string{"probe:CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 311.0, "", []string{"probe:CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 0.0, "", []string{"probe:GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 0.0, "", []string{"probe:GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 0.0, "", []string{"probe:CV"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 0.0, "", []string{"probe:CV"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 621.0, "", []string{"probe:VDDRQ"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 621.0, "", []string{"probe:VDDRQ"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 1482.0, "", []string{"probe:SYS5V"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 1482.0, "", []string{"probe:SYS5V"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := tegraCheck.processTegraStatsOutput(agXSample)
	assert.Equal(t, err, nil)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 49)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestXavierNx(t *testing.T) {

	tegraCheck := new(JetsonCheck)
	tegraCheck.Configure(nil, nil, "test")

	assert.Equal(t, tegraCheck.tegraStatsPath, "/usr/bin/tegrastats")
	mock := mocksender.NewMockSender(tegraCheck.ID())
	mock.On("Gauge", "nvidia.jetson.mem.used", 4412.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.total", 7772.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.n_lfb", 237.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.lfb", 4.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.used", 139.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.total", 3886.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.cached", 2.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 9.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 1190.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 6.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 1190.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 6.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 1190.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 5.0, "", []string{"cpu:3"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 1190.0, "", []string{"cpu:3"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 4.0, "", []string{"cpu:4"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 1190.0, "", []string{"cpu:4"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 8.0, "", []string{"cpu:5"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 1267.0, "", []string{"cpu:5"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.inactive_count", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.total_count", 6.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.emc.usage", 10.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.emc.freq", 1600.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.gpu.usage", 62.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.gpu.freq", 306.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 41.5, "", []string{"zone:AO"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 43.0, "", []string{"zone:GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 100.0, "", []string{"zone:PMIC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 41.5, "", []string{"zone:AUX"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 43.5, "", []string{"zone:CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 42.55, "", []string{"zone:thermal"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 4067.0, "", []string{"probe:VDD_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 4067.0, "", []string{"probe:VDD_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 738.0, "", []string{"probe:VDD_CPU_GPU_CV"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 738.0, "", []string{"probe:VDD_CPU_GPU_CV"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 1353.0, "", []string{"probe:VDD_SOC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 1353.0, "", []string{"probe:VDD_SOC"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := tegraCheck.processTegraStatsOutput(xavierNxSample)
	assert.Equal(t, err, nil)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 37)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

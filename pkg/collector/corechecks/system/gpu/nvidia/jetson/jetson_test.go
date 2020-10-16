// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package nvidia

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

const (
	nanoSample = "RAM 613/3964MB (lfb 634x4MB) SWAP 0/1982MB (cached 0MB) CPU [2%@102,1%@102,0%@102,0%@102] EMC_FREQ 0% GR3D_FREQ 0% PLL@39C CPU@40.5C PMIC@100C GPU@41C AO@46C thermal@41C POM_5V_IN 900/943 POM_5V_GPU 0/0 POM_5V_CPU 123/123"
	tx1Sample  = "RAM 1179/3983MB (lfb 120x4MB) IRAM 0/252kB(lfb 252kB) CPU [1%@102,4%@102,0%@102,0%@102] EMC_FREQ 7%@408 GR3D_FREQ 0%@76 APE 25 AO@42.5C CPU@37.5C GPU@39C PLL@37C Tdiode@42.75C PMIC@100C Tboard@42C thermal@38.5C VDD_IN 2532/2698 VDD_CPU 76/178 VDD_GPU 19/19"
	// TODO: Add unit test for the other devices
	//tx2Sample  = "RAM 1345/7829MB (lfb 1290x4MB) SWAP 0/512MB (cached 0MB) CPU [2%@345,off,off,off,off,off] EMC_FREQ 13%@40 GR3D_FREQ 0%@114 APE 150 BCPU@35C MCPU@35C GPU@41C PLL@35C AO@35.5C Tboard@35C Tdiode@36C PMIC@100C thermal@35.5C VDD_IN 2003/2658 VDD_CPU 320/518 VDD_GPU 400/735 VDD_SOC 400/415 VDD_WIFI 0/0 VDD_DDR 240/348"
	agXSample = "RAM 4083/31919MB (lfb 6179x4MB) SWAP 0/15959MB (cached 0MB) CPU [0%@1190,0%@1190,0%@1190,1%@1190,off,off,off,off] EMC_FREQ 0% GR3D_FREQ 0% AO@42C GPU@42C Tdiode@44.75C PMIC@100C AUX@41.5C CPU@42C thermal@41.6C Tboard@41C GPU 0/16 CPU 310/432 SOC 931/953 CV 0/0 VDDRQ 466/488 SYS5V 1522/1559"
)

func TestNano(t *testing.T) {

	tegraCheck := new(JetsonCheck)
	tegraCheck.Configure(nil, nil, "test")

	assert.Equal(t, tegraCheck.tegraStatsPath, "/usr/bin/tegrastats")

	mock := mocksender.NewMockSender(tegraCheck.ID())
	mock.On("Gauge", "nvidia.jetson.mem.used", 613.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.total", 3964.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.n_lfb", 634.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.lfb", 4.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.used", 0.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.total", 1982.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.cached", 0.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.emc.usage", 0.0, "", []string(nil)).Return().Times(1)

	mock.On("Gauge", "nvidia.jetson.gpu.usage", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 2.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 102.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 1.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 102.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 102.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:3"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 102.0, "", []string{"cpu:3"}).Return().Times(1)

	mock.On("Gauge", "nvidia.jetson.cpu.inactive_count", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.total_count", 4.0, "", []string(nil)).Return().Times(1)

	mock.On("Gauge", "nvidia.jetson.temp", 39.0, "", []string{"zone:PLL"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 40.5, "", []string{"zone:CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 100.0, "", []string{"zone:PMIC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 41.0, "", []string{"zone:GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 46.0, "", []string{"zone:AO"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 41.0, "", []string{"zone:thermal"}).Return().Times(1)

	mock.On("Gauge", "nvidia.jetson.vdd.instant", 900.0, "", []string{"probe:POM_5V_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 943.0, "", []string{"probe:POM_5V_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 0.0, "", []string{"probe:POM_5V_GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 0.0, "", []string{"probe:POM_5V_GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 123.0, "", []string{"probe:POM_5V_CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 123.0, "", []string{"probe:POM_5V_CPU"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := tegraCheck.processTegraStatsOutput(nanoSample)
	assert.Equal(t, err, nil)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 31)
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

func TestAgxXavier(t *testing.T) {

	tegraCheck := new(JetsonCheck)
	tegraCheck.Configure(nil, nil, "test")

	assert.Equal(t, tegraCheck.tegraStatsPath, "/usr/bin/tegrastats")
	mock := mocksender.NewMockSender(tegraCheck.ID())
	mock.On("Gauge", "nvidia.jetson.mem.used", 4083.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.total", 31919.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.n_lfb", 6179.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.lfb", 4.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.used", 0.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.total", 15959.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.cached", 0.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.emc.usage", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.gpu.usage", 0.0, "", []string(nil)).Return().Times(1)

	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 1190.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 1190.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 1190.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 1.0, "", []string{"cpu:3"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 1190.0, "", []string{"cpu:3"}).Return().Times(1)

	// Off cpus report 0
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:4"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 0.0, "", []string{"cpu:4"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:5"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 0.0, "", []string{"cpu:5"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:6"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 0.0, "", []string{"cpu:6"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:7"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 0.0, "", []string{"cpu:7"}).Return().Times(1)

	mock.On("Gauge", "nvidia.jetson.cpu.inactive_count", 4.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.total_count", 8.0, "", []string(nil)).Return().Times(1)

	mock.On("Gauge", "nvidia.jetson.temp", 42.0, "", []string{"zone:AO"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 42.0, "", []string{"zone:CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 42.0, "", []string{"zone:GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 41.5, "", []string{"zone:AUX"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 44.75, "", []string{"zone:Tdiode"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 100.0, "", []string{"zone:PMIC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 41.0, "", []string{"zone:Tboard"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 41.6, "", []string{"zone:thermal"}).Return().Times(1)

	mock.On("Gauge", "nvidia.jetson.vdd.instant", 931.0, "", []string{"probe:SOC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 953.0, "", []string{"probe:SOC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 310.0, "", []string{"probe:CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 432.0, "", []string{"probe:CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 0.0, "", []string{"probe:GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 16.0, "", []string{"probe:GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 0.0, "", []string{"probe:CV"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 0.0, "", []string{"probe:CV"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 466.0, "", []string{"probe:VDDRQ"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 488.0, "", []string{"probe:VDDRQ"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.instant", 1522.0, "", []string{"probe:SYS5V"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.vdd.average", 1559.0, "", []string{"probe:SYS5V"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := tegraCheck.processTegraStatsOutput(agXSample)
	assert.Equal(t, err, nil)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 47)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

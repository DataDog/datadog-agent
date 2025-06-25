// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jetson

package nvidia

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

const (
	tx1Sample         = "RAM 1179/3983MB (lfb 120x4MB) IRAM 0/252kB(lfb 252kB) CPU [1%@102,4%@102,0%@102,0%@102] EMC_FREQ 7%@408 GR3D_FREQ 0%@76 APE 25 AO@42.5C CPU@37.5C GPU@39C PLL@37C Tdiode@42.75C PMIC@100C Tboard@42C thermal@38.5C VDD_IN 2532/2698 VDD_CPU 76/178 VDD_GPU 19/19"
	tx2Sample         = "RAM 2344/7852MB (lfb 1154x4MB) SWAP 0/3926MB (cached 0MB) CPU [1%@345,off,off,1%@345,0%@345,2%@345] EMC_FREQ 4%@1600 GR3D_FREQ 0%@624 APE 150 PLL@37.5C MCPU@37.5C PMIC@100C Tboard@32C GPU@35C BCPU@37.5C thermal@36.5C Tdiode@34.25C VDD_SYS_GPU 152/152 VDD_SYS_SOC 687/687 VDD_4V0_WIFI 0/0 VDD_IN 3056/3056 VDD_SYS_CPU 152/152 VDD_SYS_DDR 883/883"
	nanoSample        = "RAM 534/3964MB (lfb 98x4MB) SWAP 5/1982MB (cached 1MB) IRAM 0/252kB(lfb 252kB) CPU [16%@204,9%@204,0%@204,0%@204] EMC_FREQ 0%@204 GR3D_FREQ 0%@76 APE 25 PLL@34C CPU@36.5C PMIC@100C GPU@36C AO@39.5C thermal@36.25C POM_5V_IN 1022/1022 POM_5V_GPU 0/0 POM_5V_CPU 204/204"
	agXSample         = "RAM 721/31927MB (lfb 7291x4MB) SWAP 0/15963MB (cached 0MB) CPU [2%@1190,0%@1190,0%@1190,0%@1190,off,off,off,off] EMC_FREQ 0%@665 GR3D_FREQ 0%@318 APE 150 MTS fg 0% bg 0% AO@37.5C GPU@37.5C Tdiode@40C PMIC@100C AUX@36C CPU@37.5C thermal@36.9C Tboard@37C GPU 0/0 CPU 311/311 SOC 932/932 CV 0/0 VDDRQ 621/621 SYS5V 1482/1482"
	xavierNxSample    = "RAM 4412/7772MB (lfb 237x4MB) SWAP 139/3886MB (cached 2MB) CPU [9%@1190,6%@1190,6%@1190,5%@1190,4%@1190,8%@1267] EMC_FREQ 10%@1600 GR3D_FREQ 62%@306 APE 150 MTS fg 0% bg 0% AO@41.5C GPU@43C PMIC@100C AUX@41.5C CPU@43.5C thermal@42.55C VDD_IN 4067/4067 VDD_CPU_GPU_CV 738/738 VDD_SOC 1353/1353"
	voltageUnitSample = "RAM 6334/15388MB (lfb 1770x4MB) SWAP 491/7694MB (cached 0MB) CPU [6%@729,9%@729,5%@729,16%@729,off,off,off,off] EMC_FREQ 0%@2133 GR3D_FREQ 0%@611 VIC_FREQ 729 APE 174 CV0@45.812C CPU@47.937C SOC2@46.093C SOC0@46.968C CV1@46.406C GPU@45.875C tj@48.875C SOC1@48.875C CV2@45.75C VDD_IN 5299mW/5299mW VDD_CPU_GPU_CV 773mW/773mW VDD_SOC 1424mW/1424mW"
	r36Sample         = "RAM 29114/30697MB (lfb 3x4MB)    SWAP 4915/15348MB (cached 1MB) CPU [3%@729,4%@729,0%@729,1%@729,0%@2201,100%@2201,1%@2201,0%@2201,100%@2201,0%@2201,0%@2201,0%@2201] EMC_FREQ 1%@2133 GR3D_FREQ 0%@[305,305] NVENC      off  NVDEC    off NVJPG off NVJPG1    off         VIC        off          OFA           off          NVDLA0    off       NVDLA1     off          PVA0_FREQ off         APE           174        cpu@53.062C soc2@48.25C soc0@48.843C  gpu@47.812C tj@53.062C soc1@48.968C    VDD_GPU_SOC 3205mW/3205mW VDD_CPU_CV 4405mW/4405mW VIN_SYS_5V0 4767mW/4767mW"
	orinSample        = `RAM 2448/62840MB (lfb 2x4MB) SWAP 0/31420MB (cached 0MB) CPU [0%@2201,0%@2201,0%@2201,0%@2201,0%@2201,0%@2201,0%@2201,0%@2201,0%@2201,0%@2201,0%@2201,0%@2201] GR3D_FREQ 0% cpu@42C soc2@37.843C soc0@39.187C gpu@37.75C tj@42C soc1@37.937C VDD_GPU_SOC 4940mW/4940mW VDD_CPU_CV 988mW/988mW VIN_SYS_5V0 4442mW/4442mW`
)

func TestNano(t *testing.T) {
	tegraCheck := new(JetsonCheck)
	mock := mocksender.NewMockSender(tegraCheck.ID())
	tegraCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	assert.Equal(t, tegraCheck.tegraStatsPath, "/usr/bin/tegrastats")

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
	mock.On("Gauge", "nvidia.jetson.power.instant", 1022.0, "", []string{"probe:POM_5V_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 1022.0, "", []string{"probe:POM_5V_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 0.0, "", []string{"probe:POM_5V_GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 0.0, "", []string{"probe:POM_5V_GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 204.0, "", []string{"probe:POM_5V_CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 204.0, "", []string{"probe:POM_5V_CPU"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := tegraCheck.processTegraStatsOutput(nanoSample)
	assert.Equal(t, err, nil)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 36)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestTX1(t *testing.T) {
	tegraCheck := new(JetsonCheck)
	mock := mocksender.NewMockSender(tegraCheck.ID())
	tegraCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	assert.Equal(t, tegraCheck.tegraStatsPath, "/usr/bin/tegrastats")

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
	mock.On("Gauge", "nvidia.jetson.power.instant", 2532.0, "", []string{"probe:VDD_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 2698.0, "", []string{"probe:VDD_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 76.0, "", []string{"probe:VDD_CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 178.0, "", []string{"probe:VDD_CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 19.0, "", []string{"probe:VDD_GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 19.0, "", []string{"probe:VDD_GPU"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := tegraCheck.processTegraStatsOutput(tx1Sample)
	assert.Equal(t, err, nil)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 35)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestTX2(t *testing.T) {
	tegraCheck := new(JetsonCheck)
	mock := mocksender.NewMockSender(tegraCheck.ID())
	tegraCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	assert.Equal(t, tegraCheck.tegraStatsPath, "/usr/bin/tegrastats")

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
	mock.On("Gauge", "nvidia.jetson.power.instant", 152.0, "", []string{"probe:VDD_SYS_GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 152.0, "", []string{"probe:VDD_SYS_GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 687.0, "", []string{"probe:VDD_SYS_SOC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 687.0, "", []string{"probe:VDD_SYS_SOC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 0.0, "", []string{"probe:VDD_4V0_WIFI"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 0.0, "", []string{"probe:VDD_4V0_WIFI"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 3056.0, "", []string{"probe:VDD_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 3056.0, "", []string{"probe:VDD_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 152.0, "", []string{"probe:VDD_SYS_CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 152.0, "", []string{"probe:VDD_SYS_CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 883.0, "", []string{"probe:VDD_SYS_DDR"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 883.0, "", []string{"probe:VDD_SYS_DDR"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := tegraCheck.processTegraStatsOutput(tx2Sample)
	assert.Equal(t, err, nil)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 45)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestAgxXavier(t *testing.T) {
	tegraCheck := new(JetsonCheck)
	mock := mocksender.NewMockSender(tegraCheck.ID())
	tegraCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	assert.Equal(t, tegraCheck.tegraStatsPath, "/usr/bin/tegrastats")
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
	mock.On("Gauge", "nvidia.jetson.power.instant", 932.0, "", []string{"probe:SOC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 932.0, "", []string{"probe:SOC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 311.0, "", []string{"probe:CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 311.0, "", []string{"probe:CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 0.0, "", []string{"probe:GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 0.0, "", []string{"probe:GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 0.0, "", []string{"probe:CV"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 0.0, "", []string{"probe:CV"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 621.0, "", []string{"probe:VDDRQ"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 621.0, "", []string{"probe:VDDRQ"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 1482.0, "", []string{"probe:SYS5V"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 1482.0, "", []string{"probe:SYS5V"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := tegraCheck.processTegraStatsOutput(agXSample)
	assert.Equal(t, err, nil)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 49)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestXavierNx(t *testing.T) {
	tegraCheck := new(JetsonCheck)
	mock := mocksender.NewMockSender(tegraCheck.ID())
	tegraCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	assert.Equal(t, tegraCheck.tegraStatsPath, "/usr/bin/tegrastats")
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
	mock.On("Gauge", "nvidia.jetson.power.instant", 4067.0, "", []string{"probe:VDD_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 4067.0, "", []string{"probe:VDD_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 738.0, "", []string{"probe:VDD_CPU_GPU_CV"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 738.0, "", []string{"probe:VDD_CPU_GPU_CV"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 1353.0, "", []string{"probe:VDD_SOC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 1353.0, "", []string{"probe:VDD_SOC"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := tegraCheck.processTegraStatsOutput(xavierNxSample)
	assert.Equal(t, err, nil)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 37)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestVoltageUnits(t *testing.T) {
	tegraCheck := new(JetsonCheck)
	mock := mocksender.NewMockSender(tegraCheck.ID())
	tegraCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	assert.Equal(t, tegraCheck.tegraStatsPath, "/usr/bin/tegrastats")

	// RAM 6334/15388MB (lfb 1770x4MB) SWAP 491/7694MB (cached 0MB)
	mock.On("Gauge", "nvidia.jetson.mem.used", 6334.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.total", 15388.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.n_lfb", 1770.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.mem.lfb", 4.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.used", 491.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.total", 7694.0*mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.swap.cached", 0.0*mb, "", []string(nil)).Return().Times(1)

	// CPU [6%@729,9%@729,5%@729,16%@729,off,off,off,off]
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 6.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 729.0, "", []string{"cpu:0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 9.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 729.0, "", []string{"cpu:1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 5.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 729.0, "", []string{"cpu:2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 16.0, "", []string{"cpu:3"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 729.0, "", []string{"cpu:3"}).Return().Times(1)
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

	// EMC_FREQ 0%@2133
	mock.On("Gauge", "nvidia.jetson.emc.usage", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.emc.freq", 2133.0, "", []string(nil)).Return().Times(1)

	// GR3D_FREQ 0%@611
	mock.On("Gauge", "nvidia.jetson.gpu.usage", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.gpu.freq", 611.0, "", []string(nil)).Return().Times(1)

	// CV0@45.812C CPU@47.937C SOC2@46.093C SOC0@46.968C CV1@46.406C GPU@45.875C tj@48.875C SOC1@48.875C CV2@45.75C
	mock.On("Gauge", "nvidia.jetson.temp", 45.812, "", []string{"zone:CV0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 47.937, "", []string{"zone:CPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 46.093, "", []string{"zone:SOC2"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 46.968, "", []string{"zone:SOC0"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 46.406, "", []string{"zone:CV1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 45.875, "", []string{"zone:GPU"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 48.875, "", []string{"zone:tj"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 48.875, "", []string{"zone:SOC1"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.temp", 45.75, "", []string{"zone:CV2"}).Return().Times(1)

	// VDD_IN 5299mW/5299mW VDD_CPU_GPU_CV 773mW/773mW VDD_SOC 1424mW/1424mW
	mock.On("Gauge", "nvidia.jetson.power.instant", 5299.0, "", []string{"probe:VDD_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 5299.0, "", []string{"probe:VDD_IN"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 773.0, "", []string{"probe:VDD_CPU_GPU_CV"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 773.0, "", []string{"probe:VDD_CPU_GPU_CV"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.instant", 1424.0, "", []string{"probe:VDD_SOC"}).Return().Times(1)
	mock.On("Gauge", "nvidia.jetson.power.average", 1424.0, "", []string{"probe:VDD_SOC"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := tegraCheck.processTegraStatsOutput(voltageUnitSample)
	assert.Equal(t, err, nil)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 44)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestR36(t *testing.T) {
	tegraCheck := new(JetsonCheck)
	mock := mocksender.NewMockSender(tegraCheck.ID())
	tegraCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	// RAM 29114/30697MB (lfb 3x4MB)    SWAP 4915/15348MB (cached 1MB)
	mock.On("Gauge", "nvidia.jetson.mem.used", 29114.0*mb, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.mem.total", 30697.0*mb, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.mem.n_lfb", 3.0, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.mem.lfb", 4.0*mb, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.swap.used", 4915.0*mb, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.swap.total", 15348.0*mb, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.swap.cached", 1.0*mb, "", []string(nil)).Return().Once()

	// CPU [3%@729,4%@729,0%@729,1%@729,0%@2201,100%@2201,1%@2201,0%@2201,100%@2201,0%@2201,0%@2201,0%@2201]
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 3.0, "", []string{"cpu:0"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 729.0, "", []string{"cpu:0"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 4.0, "", []string{"cpu:1"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 729.0, "", []string{"cpu:1"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:2"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 729.0, "", []string{"cpu:2"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 1.0, "", []string{"cpu:3"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 729.0, "", []string{"cpu:3"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:4"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:4"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 100.0, "", []string{"cpu:5"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:5"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 1.0, "", []string{"cpu:6"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:6"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:7"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:7"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 100.0, "", []string{"cpu:8"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:8"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:9"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:9"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:10"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:10"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:11"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:11"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.inactive_count", 0.0, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.total_count", 12.0, "", []string(nil)).Return().Once()

	// EMC_FREQ 1%@2133
	mock.On("Gauge", "nvidia.jetson.emc.usage", 1.0, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.emc.freq", 2133.0, "", []string(nil)).Return().Once()

	// GR3D_FREQ 0%@[305,305]
	mock.On("Gauge", "nvidia.jetson.gpu.usage", 0.0, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.gpu.freq", 305.0, "", []string{"gpc:0"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.gpu.freq", 305.0, "", []string{"gpc:1"}).Return().Once()

	// cpu@53.062C soc2@48.25C soc0@48.843C  gpu@47.812C tj@53.062C soc1@48.968C
	mock.On("Gauge", "nvidia.jetson.temp", 53.062, "", []string{"zone:cpu"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.temp", 48.25, "", []string{"zone:soc2"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.temp", 48.843, "", []string{"zone:soc0"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.temp", 47.812, "", []string{"zone:gpu"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.temp", 53.062, "", []string{"zone:tj"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.temp", 48.968, "", []string{"zone:soc1"}).Return().Once()

	// VDD_GPU_SOC 3205mW/3205mW VDD_CPU_CV 4405mW/4405mW VIN_SYS_5V0 4767mW/4767mW
	mock.On("Gauge", "nvidia.jetson.power.instant", 3205.0, "", []string{"probe:VDD_GPU_SOC"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.power.average", 3205.0, "", []string{"probe:VDD_GPU_SOC"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.power.instant", 4405.0, "", []string{"probe:VDD_CPU_CV"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.power.average", 4405.0, "", []string{"probe:VDD_CPU_CV"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.power.instant", 4767.0, "", []string{"probe:VIN_SYS_5V0"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.power.average", 4767.0, "", []string{"probe:VIN_SYS_5V0"}).Return().Once()

	mock.On("Commit").Return().Once()

	err := tegraCheck.processTegraStatsOutput(r36Sample)
	assert.NoError(t, err)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 50)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestOrin(t *testing.T) {
	tegraCheck := new(JetsonCheck)
	mock := mocksender.NewMockSender(tegraCheck.ID())
	tegraCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	// RAM 2448/62840MB (lfb 2x4MB) SWAP 0/31420MB (cached 0MB)
	mock.On("Gauge", "nvidia.jetson.mem.used", 2448.0*mb, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.mem.total", 62840.0*mb, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.mem.n_lfb", 2.0, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.mem.lfb", 4.0*mb, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.swap.used", 0.0*mb, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.swap.total", 31420.0*mb, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.swap.cached", 0.0*mb, "", []string(nil)).Return().Once()

	// CPU [0%@2201,0%@2201,0%@2201,0%@2201,0%@2201,0%@2201,0%@2201,0%@2201,0%@2201,0%@2201,0%@2201,0%@2201]
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:0"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:0"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:1"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:1"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:2"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:2"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:3"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:3"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:4"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:4"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:5"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:5"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:6"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:6"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:7"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:7"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:8"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:8"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:9"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:9"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:10"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:10"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.usage", 0.0, "", []string{"cpu:11"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.freq", 2201.0, "", []string{"cpu:11"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.inactive_count", 0.0, "", []string(nil)).Return().Once()
	mock.On("Gauge", "nvidia.jetson.cpu.total_count", 12.0, "", []string(nil)).Return().Once()

	// GR3D_FREQ 0%
	mock.On("Gauge", "nvidia.jetson.gpu.usage", 0.0, "", []string(nil)).Return().Once()

	// cpu@42C soc2@37.843C soc0@39.187C gpu@37.75C tj@42C soc1@37.937C
	mock.On("Gauge", "nvidia.jetson.temp", 42.0, "", []string{"zone:cpu"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.temp", 37.843, "", []string{"zone:soc2"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.temp", 39.187, "", []string{"zone:soc0"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.temp", 37.75, "", []string{"zone:gpu"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.temp", 42.0, "", []string{"zone:tj"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.temp", 37.937, "", []string{"zone:soc1"}).Return().Once()

	// VDD_GPU_SOC 4940mW/4940mW VDD_CPU_CV 988mW/988mW VIN_SYS_5V0 4442mW/4442mW
	mock.On("Gauge", "nvidia.jetson.power.instant", 4940.0, "", []string{"probe:VDD_GPU_SOC"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.power.average", 4940.0, "", []string{"probe:VDD_GPU_SOC"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.power.instant", 988.0, "", []string{"probe:VDD_CPU_CV"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.power.average", 988.0, "", []string{"probe:VDD_CPU_CV"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.power.instant", 4442.0, "", []string{"probe:VIN_SYS_5V0"}).Return().Once()
	mock.On("Gauge", "nvidia.jetson.power.average", 4442.0, "", []string{"probe:VIN_SYS_5V0"}).Return().Once()

	mock.On("Commit").Return().Once()

	err := tegraCheck.processTegraStatsOutput(orinSample)
	assert.NoError(t, err)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 46)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

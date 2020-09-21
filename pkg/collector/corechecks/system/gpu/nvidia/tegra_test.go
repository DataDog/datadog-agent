// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package nvidia

import (
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

const (
	nanoSample = "RAM 613/3964MB (lfb 634x4MB) SWAP 0/1982MB (cached 0MB) CPU [2%@102,1%@102,0%@102,0%@102] EMC_FREQ 0% GR3D_FREQ 0% PLL@39C CPU@40.5C PMIC@100C GPU@41C AO@46C thermal@41C POM_5V_IN 900/943 POM_5V_GPU 0/0 POM_5V_CPU 123/123"
	tx1Sample = "RAM 1179/3983MB (lfb 120x4MB) IRAM 0/252kB(lfb 252kB) CPU [1%@102,4%@102,0%@102,0%@102] EMC_FREQ 7%@408 GR3D_FREQ 0%@76 APE 25 AO@42.5C CPU@37.5C GPU@39C PLL@37C Tdiode@42.75C PMIC@100C Tboard@42C thermal@38.5C VDD_IN 2532/2698 VDD_CPU 76/178 VDD_GPU 19/19"
	tx2Sample = "RAM 1345/7829MB (lfb 1290x4MB) SWAP 0/512MB (cached 0MB) CPU [2%@345,off,off,off,off,off] EMC_FREQ 13%@40 GR3D_FREQ 0%@114 APE 150 BCPU@35C MCPU@35C GPU@41C PLL@35C AO@35.5C Tboard@35C Tdiode@36C PMIC@100C thermal@35.5C VDD_IN 2003/2658 VDD_CPU 320/518 VDD_GPU 400/735 VDD_SOC 400/415 VDD_WIFI 0/0 VDD_DDR 240/348"
	agXSample = "RAM 1903/15692MB (lfb 3251x4MB) CPU [1%@1190,1%@1190,2%@1190,0%@1190,0%@1190,0%@1190,0%@1190,0%@1190] EMC_FREQ 0% GR3D_FREQ 0% AO@32.5C GPU@32C Tboard@32C Tdiode@34.75C AUX@31.5C CPU@33.5C thermal@32.9C PMIC@100C GPU 0/0 CPU 216/216 SOC 864/864 CV 0/0 VDDRQ 144/144 SYS5V 1889/1889"
)

func testNanoTegraStats(t *testing.T) {

	tegraCheck := new(TegraCheck)
	tegraCheck.Configure(nil, nil, "test")

	assert.Equal(t, tegraCheck.tegraStatsPath, "/usr/bin/tegrastats")

	mock := mocksender.NewMockSender(tegraCheck.ID())
	mock.On("Gauge", "system.gpu.mem.used", 613.0 * mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.gpu.mem.total", 3964.0 * mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.gpu.mem.n_lfb", 634.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.gpu.mem.lfb", 4.0 * mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.gpu.swap.used", 0.0 * mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.gpu.swap.total", 1982.0 * mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.gpu.swap.cached", 0.0 * mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.gpu.emc.usage", 0.0, "", []string(nil)).Return().Times(1)
	// Freq is not sent if not found
	//mock.On("Gauge", "system.gpu.emc.freq", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.gpu.usage", 0.0, "", []string(nil)).Return().Times(1)
	// Freq is not sent if not found
	//mock.On("Gauge", "system.gpu.freq", 39.0, "", []string(nil)).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := tegraCheck.processTegraStatsOutput(nanoSample)
	assert.Equal(t, err, nil)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 9)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func testTx1TegraStats(t *testing.T) {

	tegraCheck := new(TegraCheck)
	tegraCheck.Configure(nil, nil, "test")

	assert.Equal(t, tegraCheck.tegraStatsPath, "/usr/bin/tegrastats")

	mock := mocksender.NewMockSender(tegraCheck.ID())
	mock.On("Gauge", "system.gpu.mem.used", 1179.0 * mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.gpu.mem.total", 3983.0 * mb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.gpu.mem.n_lfb", 120.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.gpu.mem.lfb", 4.0 * mb, "", []string(nil)).Return().Times(1)
	//mock.On("Gauge", "system.gpu.iram.used", 0.0 * kb, "", []string(nil)).Return().Times(1)
	//mock.On("Gauge", "system.gpu.iram.total", 252.0 * kb, "", []string(nil)).Return().Times(1)
	//mock.On("Gauge", "system.gpu.iram.lfb", 252.0 * kb, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.gpu.emc.usage", 7.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.gpu.emc.freq", 408, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.gpu.usage", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.gpu.freq", 76.0, "", []string(nil)).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := tegraCheck.processTegraStatsOutput(tx1Sample)
	assert.Equal(t, err, nil)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 8)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

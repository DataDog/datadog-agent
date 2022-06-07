// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package network

import (
	"testing"
	"math"
	// "strings"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	// ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	netcfg "github.com/DataDog/datadog-agent/pkg/network/config"
)

func TestCalcDriverSizeEx(t *testing.T) {

	//    | net.driver_buffer_size | net.driver_buffer_entries |                 use                    |
	// 1. |          Default       |            Default        |    net.driver_buffer_entries * PerFlow |
	// 2. |          Default       |            !Default       |    net.driver_buffer_entries * PerFlow |
	// 3. |          !Default      |            Default        |    net.driver_buffer_size * PerFlow    |
	// 4. |          !Default      |            !Default       |    net.driver_buffer_entries * PerFlow |

	res := 0
	exp := 0
	
	// make initial config
	initialConfig := netcfg.New()


	// 1.
	exp = config.DefaultFlowEntries * driver.PerFlowDataSize
	res = calcDriverBufferSize(initialConfig.DriverBufferSize, initialConfig.DriverBufferEntries)
	assert.Equal(t, res, exp)

	// 2.
	initialConfig = netcfg.New()
	initialConfig.DriverBufferEntries = config.DefaultFlowEntries + 5
	exp = initialConfig.DriverBufferEntries*driver.PerFlowDataSize
	res = calcDriverBufferSize(initialConfig.DriverBufferSize, initialConfig.DriverBufferEntries)
	assert.Equal(t, res, exp)

	// 3.
	initialConfig = netcfg.New()
	initialConfig.DriverBufferSize = config.DefaultDriverBufferSize + 5
	exp = int(math.Ceil(float64(initialConfig.DriverBufferSize) / float64(driver.PerFlowDataSize)))*driver.PerFlowDataSize
	res = calcDriverBufferSize(initialConfig.DriverBufferSize, initialConfig.DriverBufferEntries)
	assert.Equal(t, res, exp)

	// 4.
	initialConfig = netcfg.New()
	initialConfig.DriverBufferEntries = config.DefaultFlowEntries + 5
	initialConfig.DriverBufferSize = config.DefaultDriverBufferSize + 5
	exp = initialConfig.DriverBufferEntries * driver.PerFlowDataSize
	res = calcDriverBufferSize(initialConfig.DriverBufferSize, initialConfig.DriverBufferEntries)
	assert.Equal(t, res, exp)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package network

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	netcfg "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
)

func TestCalcDriverSize(t *testing.T) {

	//    | net.driver_buffer_size | net.driver_buffer_entries |                 use                    |
	// 1. |          Default       |            Default        |    net.driver_buffer_entries * PerFlow |
	// 2. |          Default       |            !Default       |    net.driver_buffer_entries * PerFlow |
	// 3. |          !Default      |            Default        |    net.driver_buffer_size * PerFlow    |
	// 4. |          !Default      |            !Default       |    net.driver_buffer_entries * PerFlow |

	const sizeModifier int = 5
	res := 0
	exp := 0

	// make initial config
	initialConfig := netcfg.New()

	// 1. All defaults
	exp = config.DefaultFlowEntries * driver.PerFlowDataSize
	res = calcDriverBufferSize(initialConfig.DriverBufferSize, initialConfig.DriverBufferEntries)
	assert.Equal(t, res, exp)

	// 2. buffer_size default, entries set
	initialConfig = netcfg.New()
	initialConfig.DriverBufferEntries = config.DefaultFlowEntries + sizeModifier
	exp = initialConfig.DriverBufferEntries * driver.PerFlowDataSize
	res = calcDriverBufferSize(initialConfig.DriverBufferSize, initialConfig.DriverBufferEntries)
	assert.Equal(t, res, exp)

	// 3. buffer_size set, entries default
	initialConfig = netcfg.New()
	initialConfig.DriverBufferSize = config.DefaultDriverBufferSize + sizeModifier
	exp = int(math.Ceil(float64(initialConfig.DriverBufferSize)/float64(driver.PerFlowDataSize))) * driver.PerFlowDataSize
	res = calcDriverBufferSize(initialConfig.DriverBufferSize, initialConfig.DriverBufferEntries)
	assert.Equal(t, res, exp)

	// 4. buffer_size set, entries set
	initialConfig = netcfg.New()
	initialConfig.DriverBufferEntries = config.DefaultFlowEntries + sizeModifier
	initialConfig.DriverBufferSize = config.DefaultDriverBufferSize + sizeModifier
	exp = initialConfig.DriverBufferEntries * driver.PerFlowDataSize
	res = calcDriverBufferSize(initialConfig.DriverBufferSize, initialConfig.DriverBufferEntries)
	assert.Equal(t, res, exp)
}

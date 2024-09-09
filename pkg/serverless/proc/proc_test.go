// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package proc

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPidListInvalid(t *testing.T) {
	pids := getPidList("/incorrect/folder")
	assert.Equal(t, 0, len(pids))
}

func TestGetPidListValid(t *testing.T) {
	pids := getPidList("./testData")
	sort.Ints(pids)
	assert.Equal(t, 2, len(pids))
	assert.Equal(t, 13, pids[0])
	assert.Equal(t, 142, pids[1])
}

func TestSearchProcsForEnvVariableFromPidIncorrect(t *testing.T) {
	envVars := getEnvVariablesFromPid("./testData", 999)
	assert.Equal(t, 0, len(envVars))
}

func TestSearchProcsForEnvVariableFromPidCorrect(t *testing.T) {
	envVars := getEnvVariablesFromPid("./testData", 13)
	assert.Equal(t, "value0", envVars["env0"])
	assert.Equal(t, "value1", envVars["env1"])
	assert.Equal(t, "AWS_Lambda_nodejs14.x", envVars["AWS_EXECUTION_ENV"])
	assert.Equal(t, "value3", envVars["env3"])
	assert.Equal(t, 4, len(envVars))
}

func TestSearchProcsForEnvVariableFound(t *testing.T) {
	result := SearchProcsForEnvVariable("./testData", "env1")
	expected := []string{"value1"}
	assert.Equal(t, 1, len(result))
	assert.Equal(t, expected[0], result[0])
}
func TestSearchProcsForEnvVariableNotFound(t *testing.T) {
	result := SearchProcsForEnvVariable("./testData", "xxx")
	assert.Equal(t, 0, len(result))
}

func TestGetCPUData(t *testing.T) {
	path := "./testData/stat/valid_stat"
	cpuData, err := getCPUData(path)
	assert.Nil(t, err)
	assert.Equal(t, float64(23370), cpuData.TotalUserTimeMs)
	assert.Equal(t, float64(1880), cpuData.TotalSystemTimeMs)
	assert.Equal(t, float64(178380), cpuData.TotalIdleTimeMs)
	assert.Equal(t, 2, len(cpuData.IndividualCPUIdleTimes))
	assert.Equal(t, float64(91880), cpuData.IndividualCPUIdleTimes["cpu0"])
	assert.Equal(t, float64(86490), cpuData.IndividualCPUIdleTimes["cpu1"])

	path = "./testData/stat/invalid_stat_non_numerical_value_1"
	cpuData, err = getCPUData(path)
	assert.NotNil(t, err)
	assert.Nil(t, cpuData)

	path = "./testData/stat/invalid_stat_non_numerical_value_2"
	cpuData, err = getCPUData(path)
	assert.NotNil(t, err)
	assert.Nil(t, cpuData)

	path = "./testData/stat/invalid_stat_malformed_first_line"
	cpuData, err = getCPUData(path)
	assert.NotNil(t, err)
	assert.Nil(t, cpuData)

	path = "./testData/stat/invalid_stat_malformed_per_cpu_line"
	cpuData, err = getCPUData(path)
	assert.NotNil(t, err)
	assert.Nil(t, cpuData)

	path = "./testData/stat/invalid_stat_missing_cpun_data"
	cpuData, err = getCPUData(path)
	assert.NotNil(t, err)
	assert.Nil(t, cpuData)

	path = "./testData/stat/nonexistent_stat"
	cpuData, err = getCPUData(path)
	assert.NotNil(t, err)
	assert.Nil(t, cpuData)
}

func TestGetUptime(t *testing.T) {
	path := "./testData/uptime/valid_uptime"
	uptime, err := getUptime(path)
	assert.Nil(t, err)
	assert.Equal(t, float64(3213103123000), uptime)

	path = "./testData/uptime/invalid_data_uptime"
	uptime, err = getUptime(path)
	assert.NotNil(t, err)
	assert.Equal(t, float64(0), uptime)

	path = "./testData/uptime/malformed_uptime"
	uptime, err = getUptime(path)
	assert.NotNil(t, err)
	assert.Equal(t, float64(0), uptime)

	path = "./testData/uptime/nonexistent_uptime"
	uptime, err = getUptime(path)
	assert.NotNil(t, err)
	assert.Equal(t, float64(0), uptime)
}

func TestGetNetworkData(t *testing.T) {
	path := "./testData/net/valid_dev"
	networkData, err := getNetworkData(path)
	assert.Nil(t, err)
	assert.Equal(t, float64(180), networkData.RxBytes)
	assert.Equal(t, float64(254), networkData.TxBytes)

	path = "./testData/net/invalid_dev_malformed"
	networkData, err = getNetworkData(path)
	assert.NotNil(t, err)
	assert.Nil(t, networkData)

	path = "./testData/net/invalid_dev_non_numerical_value"
	networkData, err = getNetworkData(path)
	assert.NotNil(t, err)
	assert.Nil(t, networkData)

	path = "./testData/net/missing_interface_dev"
	networkData, err = getNetworkData(path)
	assert.NotNil(t, err)
	assert.Nil(t, networkData)

	path = "./testData/net/nonexistent_dev"
	networkData, err = getNetworkData(path)
	assert.NotNil(t, err)
	assert.Nil(t, networkData)
}

func TestGetFileDescriptorMaxData(t *testing.T) {
	path := "./testData/file-descriptor/valid"
	fileDescriptorMaxData, err := getFileDescriptorMaxData(path)
	assert.Nil(t, err)
	assert.Equal(t, float64(1024), fileDescriptorMaxData.MaximumFileHandles)

	path = "./testData/file-descriptor/invalid_malformed"
	fileDescriptorMaxData, err = getFileDescriptorMaxData(path)
	assert.NotNil(t, err)
	assert.Nil(t, fileDescriptorMaxData)

	path = "./testData/file-descriptor/invalid_missing"
	fileDescriptorMaxData, err = getFileDescriptorMaxData(path)
	assert.NotNil(t, err)
	assert.Nil(t, fileDescriptorMaxData)
}

func TestGetFileDescriptorUseData(t *testing.T) {
	path := "./testData/file-descriptor/valid"
	fileDescriptorUseData, err := getFileDescriptorUseData(path)
	assert.Nil(t, err)
	assert.Equal(t, float64(5), fileDescriptorUseData.UseFileHandles)

	path = "./testData/file-descriptor/invalid_missing"
	fileDescriptorUseData, err = getFileDescriptorUseData(path)
	assert.NotNil(t, err)
	assert.Nil(t, fileDescriptorUseData)
}

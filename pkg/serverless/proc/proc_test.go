// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package proc

import (
	"os"
	"sort"
	"testing"

	"gotest.tools/assert"
)

func TestGetPidListInvalid(t *testing.T) {
	pids := getPidList("/incorrect/folder")
	assert.Equal(t, 0, len(pids))
}

func TestGetPidListValid(t *testing.T) {
	pids := getPidList("./testData/")
	sort.Ints(pids)
	assert.Equal(t, 2, len(pids))
	assert.Equal(t, 13, pids[0])
	assert.Equal(t, 142, pids[1])
}

func TestGetEnvVariablesFromPidIncorrect(t *testing.T) {
	envVars := getEnvVariablesFromPid("./testData/", 999)
	assert.Equal(t, 0, len(envVars))
}

func TestGetEnvVariablesFromPidCorrect(t *testing.T) {
	fakeEnviron := []byte("env0=value0\000env1=value1\000env2=value2\000env3=value3")
	err := os.WriteFile("./testData/13/environ", fakeEnviron, 0644)
	assert.NilError(t, err)

	envVars := getEnvVariablesFromPid("./testData/", 13)
	assert.Equal(t, "value0", envVars["env0"])
	assert.Equal(t, "value1", envVars["env1"])
	assert.Equal(t, "value2", envVars["env2"])
	assert.Equal(t, "value3", envVars["env3"])
	assert.Equal(t, 4, len(envVars))
}

func TestGetEnvVariableFound(t *testing.T) {
	fakeEnviron := []byte("env0=value0\000env1=value1\000env2=value2\000env3=value3")
	err := os.WriteFile("./testData/13/environ", fakeEnviron, 0644)
	assert.NilError(t, err)

	result := GetEnvVariable("./testData/", "env1")
	assert.Equal(t, "value1", result)
}
func TestGetEnvVariableNotFound(t *testing.T) {
	fakeEnviron := []byte("env0=value0\000env1=value1\000env2=value2\000env3=value3")
	err := os.WriteFile("./testData/13/environ", fakeEnviron, 0644)
	assert.NilError(t, err)

	result := GetEnvVariable("./testData/", "xxx")
	assert.Equal(t, "", result)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInAzureAppServices(t *testing.T) {
	os.Setenv(RunZip, " ")
	isLinuxAzure := InAzureAppServices()
	os.Unsetenv(RunZip)

	os.Setenv(AppLogsTrace, " ")
	isWindowsAzure := InAzureAppServices()
	os.Unsetenv(AppLogsTrace)

	isNotAzure := InAzureAppServices()

	assert.True(t, isLinuxAzure)
	assert.True(t, isWindowsAzure)
	assert.False(t, isNotAzure)
}

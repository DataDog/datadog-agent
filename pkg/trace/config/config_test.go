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

const (
	FxVersion    = "APPSVC_RUN_ZIP"
	AppLogsTrace = "WEBSITE_APPSERVICEAPPLOGS_TRACE_ENABLED"
)

func TestInAzureAppServices(t *testing.T) {
	os.Setenv(FxVersion, " ")
	isLinuxAzure := inAzureAppServices()
	os.Unsetenv(FxVersion)

	os.Setenv(AppLogsTrace, " ")
	isWindowsAzure := inAzureAppServices()
	os.Unsetenv(AppLogsTrace)

	isNotAzure := inAzureAppServices()

	assert.True(t, isLinuxAzure)
	assert.True(t, isWindowsAzure)
	assert.False(t, isNotAzure)
}

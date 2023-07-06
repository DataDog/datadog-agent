// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInAzureAppServices(t *testing.T) {
	isLinuxAzure := inAzureAppServices(func(s string) string { return "APPSVC_RUN_ZIP" })
	isWindowsAzure := inAzureAppServices(func(s string) string { return "WEBSITE_APPSERVICEAPPLOGS_TRACE_ENABLED" })
	isNotAzure := inAzureAppServices(func(s string) string { return "" })
	assert.True(t, isLinuxAzure)
	assert.True(t, isWindowsAzure)
	assert.False(t, isNotAzure)
}

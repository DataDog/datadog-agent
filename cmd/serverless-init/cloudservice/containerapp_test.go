// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetContainerAppTags(t *testing.T) {
	service := &ContainerApp{}

	t.Setenv("CONTAINER_APP_NAME", "test_app_name")
	t.Setenv("CONTAINER_APP_ENV_DNS_SUFFIX", "test.bluebeach.eastus.azurecontainerapps.io")
	t.Setenv("CONTAINER_APP_REVISION", "test_revision")

	tags := service.GetTags()

	assert.Equal(t, map[string]string{
		"app_name":   "test_app_name",
		"origin":     "containerapp",
		"region":     "eastus",
		"revision":   "test_revision",
		"_dd.origin": "containerapp",
	}, tags)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLinuxAppServiceTags(t *testing.T) {
	service := &AppService{}

	t.Setenv(cloudservice.WebsiteName, "test_site_name")
	t.Setenv(cloudservice.RegionName, "eastus")
	t.Setenv(cloudservice.RunZip, "false")

	tags := service.GetTags()
	tags[cloudservice.AasOperatingSystem] = "linux"
	tags[cloudservice.AasRuntime] = "test_runtime"
	tags[cloudservice.AasInstanceName] = "test_instance_name"

	assert.Equal(t, map[string]string{
		"app_name":                "test_site_name",
		"origin":                  "appservice",
		"region":                  "eastus",
		"_dd.origin":              "appservice",
		cloudservice.AasInstanceID:      "unknown",
		cloudservice.AasInstanceName:    "test_instance_name",
		cloudservice.AasOperatingSystem: "linux",
		cloudservice.AasRuntime:         "test_runtime",
		cloudservice.AasResourceGroup:   "",
		cloudservice.AasResourceID:      "",
		cloudservice.AasSiteKind:        "app",
		cloudservice.AasSiteName:        "test_site_name",
		cloudservice.AasSiteType:        "app",
		cloudservice.AasSubscriptionID:  "",
	}, tags)
}

func TestGetWindowsAppServiceTags(t *testing.T) {
	service := &AppService{}

	t.Setenv(cloudservice.WebsiteName, "test_site_name")
	t.Setenv(cloudservice.RegionName, "eastus")
	t.Setenv(cloudservice.AppLogsTrace, "false")

	tags := service.GetTags()
	tags[cloudservice.AasOperatingSystem] = "windows"
	tags[cloudservice.AasRuntime] = "test_runtime"
	tags[cloudservice.AasInstanceName] = "test_instance_name"

	assert.Equal(t, map[string]string{
		"app_name":                "test_site_name",
		"origin":                  "appservice",
		"region":                  "eastus",
		"_dd.origin":              "appservice",
		cloudservice.AasInstanceID:      "unknown",
		cloudservice.AasInstanceName:    "test_instance_name",
		cloudservice.AasOperatingSystem: "windows",
		cloudservice.AasRuntime:         "test_runtime",
		cloudservice.AasResourceGroup:   "",
		cloudservice.AasResourceID:      "",
		cloudservice.AasSiteKind:        "app",
		cloudservice.AasSiteName:        "test_site_name",
		cloudservice.AasSiteType:        "app",
		cloudservice.AasSubscriptionID:  "",
	}, tags)
}

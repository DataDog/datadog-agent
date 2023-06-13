// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAppServiceTags(t *testing.T) {
	service := &AppService{}

	t.Setenv("WEBSITE_SITE_NAME", "test_site_name")
	t.Setenv("REGION_NAME", "eastus")
	t.Setenv("APPSVC_RUN_ZIP", "false")

	tags := service.GetTags()
	tags["aas.environment.os"] = "test_os"
	tags["aas.environment.runtime"] = "test_runtime"
	tags["aas.environment.instance_name"] = "test_instance_name"

	assert.Equal(t, map[string]string{
		"app_name":                      "test_site_name",
		"origin":                        "appservice",
		"region":                        "eastus",
		"_dd.origin":                    "appservice",
		"aas.environment.instance_id":   "",
		"aas.environment.instance_name": "test_instance_name",
		"aas.environment.os":            "test_os",
		"aas.environment.runtime":       "test_runtime",
		"aas.resource.group":            "",
		"aas.resource.id":               "",
		"aas.site.kind":                 "app",
		"aas.site.name":                 "test_site_name",
		"aas.site.type":                 "app",
		"aas.subscription.id":           "",
	}, tags)
}

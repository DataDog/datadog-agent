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

	assert.Equal(t, map[string]string{
		"app_name":   "test_site_name",
		"origin":     "appservice",
		"region":     "eastus",
		"_dd.origin": "appservice",
	}, tags)
}

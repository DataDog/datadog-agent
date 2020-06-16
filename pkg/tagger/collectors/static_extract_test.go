// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package collectors

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestGetTagInfo(t *testing.T) {
	mockConfig := config.Mock()
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3:value:2:value:3", "tag4:"})
	defer mockConfig.Set("tags", nil)

	expected := []*TagInfo{
		{
			Source: "static",
			Entity: "some_entity_name",
			LowCardTags: []string{
				"tag1:value1",
				"tag3:value:2:value:3",
			},
			DeleteEntity: false,
		},
	}

	c := &StaticCollector{}
	c.ddTagsEnvVar = config.Datadog.GetStringSlice("tags")

	result := c.getTagInfo("some_entity_name")
	assertTagInfoListEqual(t, result, expected)
}

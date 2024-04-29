// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/config"
)

func addResourceTags(t *utils.TagList, m map[string]string) {
	for k, v := range m {
		// Ignore non user-defined tags
		if strings.HasPrefix(k, "aws:") {
			continue
		}

		if config.Datadog.GetBool("ecs_resource_tags_replace_colon") {
			k = strings.ReplaceAll(k, ":", "_")
		}

		t.AddLow(strings.ToLower(k), strings.ToLower(v))
	}
}

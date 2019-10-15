// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package collectors

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
)

func addResourceTags(t *utils.TagList, m map[string]string) {
	for k, v := range m {
		// Ignore non user-defined tags
		if strings.HasPrefix(k, "aws:") {
			continue
		}
		t.AddLow(strings.ToLower(k), strings.ToLower(v))
	}
}

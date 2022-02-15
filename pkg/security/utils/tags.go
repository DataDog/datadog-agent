// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"strings"
)

// GetTagValue returns the value of the given tag in the given list
func GetTagValue(tagName string, tags []string) string {
	for _, tag := range tags {
		kv := strings.SplitN(tag, ":", 2)
		if len(kv) != 2 {
			continue
		}
		if kv[0] == tagName {
			return kv[1]
		}
	}
	return ""
}

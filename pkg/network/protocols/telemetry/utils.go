// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
)

// Example: given {"_opt3", "tag:a", "_opt2", "tag:b", "_opt1", "tag:c"}
// The return value will be:
// {"tag:a", "tag:b", "tag:c"}
// {"_opt1", "_opt2", "_opt3"}
func splitTagsAndOptions(all []string) (tags, opts sets.Set[string]) {
	if len(all) == 0 {
		return
	}
	tags = sets.New[string]()
	opts = sets.New[string]()

	for _, s := range all {
		if strings.HasPrefix(s, optPrefix) {
			opts.Insert(s)
		} else {
			tags.Insert(s)
		}
	}

	return
}

// Example: given "usm.http.hits"
// The return value will be: "usm.http" and "hits"
func splitName(m metric) (namespace, name string) {
	fullName := m.base().name
	separatorPos := strings.LastIndex(fullName, ".")
	if separatorPos < 0 {
		return "", fullName
	}

	return fullName[:separatorPos], fullName[separatorPos+1:]
}

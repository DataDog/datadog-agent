// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filters

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// FilteredByTags reports whether a trace must be dropped because its root span fails the
// apm_config.filter_tags and apm_config.filter_tags_regex rules.
func FilteredByTags(root *pb.Span, require, reject []*config.Tag, requireRegex, rejectRegex []*config.TagRegex) bool {
	for _, tag := range reject {
		if v, ok := root.Meta[tag.K]; ok && (tag.V == "" || v == tag.V) {
			return true
		}
	}
	for _, tag := range rejectRegex {
		if v, ok := root.Meta[tag.K]; ok && (tag.V == nil || tag.V.MatchString(v)) {
			return true
		}
	}
	for _, tag := range require {
		v, ok := root.Meta[tag.K]
		if !ok || (tag.V != "" && v != tag.V) {
			return true
		}
	}
	for _, tag := range requireRegex {
		v, ok := root.Meta[tag.K]
		if !ok || (tag.V != nil && !tag.V.MatchString(v)) {
			return true
		}
	}
	return false
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package powershell

import (
	"strconv"
	"strings"
)

// toFloat coerces a JSON-decoded cmdlet property value into a float64 suitable
// for a metric. Enums arrive as their integer value (float64); booleans map to
// 1/0; numeric strings are parsed. Returns false when the value cannot be
// represented as a number.
func toFloat(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// tagValue renders a property value as a tag value string.
func tagValue(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return ""
	}
}

// buildTags assembles the tag list for a single output row: tag_by properties
// (with their aliases), fixed tags, and tag_queries join results. joinResults
// is indexed in the same order as inst.TagQueries; each maps a link-target
// value to the joined target-property value.
func buildTags(inst *instanceConfig, row map[string]interface{}, joinResults []map[string]string) []string {
	var tags []string

	for i := range inst.TagBy {
		tb := &inst.TagBy[i]
		if v := tagValue(row[tb.Property]); v != "" {
			tags = append(tags, tb.Alias+":"+v)
		}
	}

	tags = append(tags, inst.Tags...)

	for i := range inst.TagQueries {
		q := &inst.TagQueries[i]
		if i >= len(joinResults) || joinResults[i] == nil {
			continue
		}
		srcVal := tagValue(row[q.LinkSourceProperty])
		if srcVal == "" {
			continue
		}
		if v, ok := joinResults[i][srcVal]; ok && v != "" {
			tags = append(tags, q.Alias+":"+v)
		}
	}

	return tags
}

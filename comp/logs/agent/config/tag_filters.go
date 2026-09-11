// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/comp/logs-library/tagfilter"
)

// TagFilters defines include/exclude tag patterns, either global (logs_config.tag_filters)
// or per-source (the tag_filters field on LogsConfig).
type TagFilters struct {
	Include []string `mapstructure:"include" json:"include" yaml:"include"`
	Exclude []string `mapstructure:"exclude" json:"exclude" yaml:"exclude"`
}

// Compile builds the matcher for f. A nil f compiles like an empty filter, and, like
// tagfilter.Compile, this never fails: a malformed pattern is reported, not returned as an error.
func (f *TagFilters) Compile() (*tagfilter.Filters, tagfilter.Report) {
	if f == nil {
		return tagfilter.Compile(nil, nil)
	}
	return tagfilter.Compile(f.Include, f.Exclude)
}

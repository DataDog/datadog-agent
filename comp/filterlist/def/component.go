// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filterlist defines a component to handle the metric and tag filterlist
// including any updates from RC. The filter list can be configured to remove metrics
// and tags from metrics as they are being processed to prevent them from being sent
// to DataDog.
package filterlist

// team: agent-metric-pipelines

import utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"

type Component interface {
	OnUpdateMetricFilterList(func(utilstrings.Matcher, utilstrings.Matcher))
	OnUpdateTagFilterList(func(TagMatcher))
	GetMetricFilterList() utilstrings.Matcher
	GetTagFilterList() TagMatcher
}

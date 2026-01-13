// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filterlist defines a component to handle the metric and tag filterlist
// including any updates from RC.
package filterlist

import utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"

type Component interface {
	OnUpdateMetricFilterList(func(*utilstrings.Matcher, *utilstrings.Matcher))
}

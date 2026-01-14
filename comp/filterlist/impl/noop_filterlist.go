// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

// NoopFilterList is a Noop FilterList to be used for tests.
type NoopFilterList struct{}

func NewNoopFilterList() filterlist.Component {
	return &NoopFilterList{}
}

// OnUpdateMetricFilterList does nothing.
func (*NoopFilterList) OnUpdateMetricFilterList(_ func(utilstrings.Matcher, utilstrings.Matcher)) {
}

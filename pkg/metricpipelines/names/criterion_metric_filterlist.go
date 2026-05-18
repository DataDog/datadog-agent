// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package names

import (
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

type metricFilterListCriterion struct{}

func (metricFilterListCriterion) id() CriterionID {
	return CriterionMetricFilterList
}

func (metricFilterListCriterion) active(_ pkgconfigmodel.Reader) bool {
	return true
}

func (metricFilterListCriterion) matchers(_ pkgconfigmodel.Reader, filterList filterlist.Component) (utilstrings.Matcher, utilstrings.Matcher) {
	return filterList.GetMetricFilterList(), utilstrings.Matcher{}
}

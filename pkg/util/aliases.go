// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"github.com/DataDog/datadog-agent/pkg/util/sort"
	"github.com/DataDog/datadog-agent/pkg/util/statictags"
)

// SortUniqInPlace alias
var SortUniqInPlace = sort.UniqInPlace

var (
	GetStaticTagsSlice = statictags.GetStaticTagsSlice
	GetStaticTags      = statictags.GetStaticTags
)

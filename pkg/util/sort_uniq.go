// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import "github.com/DataDog/datadog-agent/pkg/util/util_sort"

// SortUniqInPlace sorts and remove duplicates from elements in place
// The returned slice is a subslice of elements
var SortUniqInPlace = util_sort.SortUniqInPlace

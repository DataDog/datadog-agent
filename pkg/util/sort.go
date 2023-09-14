// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import "github.com/DataDog/datadog-agent/pkg/util/util_sort"

// InsertionSortThreshold is the slice size after which we should consider
// using the stdlib sort method instead of the InsertionSort implemented below.
const InsertionSortThreshold = 40

// InsertionSort sorts in-place the given elements, not doing any allocation.
// It is very efficient for on slices but if memory allocation is not an issue,
// consider using the stdlib `sort.Sort` method on slices having a size > InsertionSortThreshold.
// See `pkg/util/sort_benchmarks_note.md` for more details.
var InsertionSort = util_sort.InsertionSort

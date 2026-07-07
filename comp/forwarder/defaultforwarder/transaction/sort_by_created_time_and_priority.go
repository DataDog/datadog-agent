// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transaction

import "sort"

// SortByCreatedTimeAndPriority sorts transactions by priority (highest first) and,
// for transactions with equal priority, by creation time (newest first).
func SortByCreatedTimeAndPriority(transactions []Transaction) {
	sort.Sort(byCreatedTimeAndPriority(transactions))
}

type byCreatedTimeAndPriority []Transaction

func (v byCreatedTimeAndPriority) Len() int      { return len(v) }
func (v byCreatedTimeAndPriority) Swap(i, j int) { v[i], v[j] = v[j], v[i] }
func (v byCreatedTimeAndPriority) Less(i, j int) bool {
	if v[i].GetPriority() != v[j].GetPriority() {
		return v[i].GetPriority() > v[j].GetPriority()
	}
	return v[i].GetCreatedAt().After(v[j].GetCreatedAt())
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transaction

import "sort"

// SortByCreatedTimeAndPriority sorts transactions by creation time and priority
type SortByCreatedTimeAndPriority struct {
	HighPriorityFirst bool
}

// Sort sorts transactions by creation time and priority
func (s SortByCreatedTimeAndPriority) Sort(transactions []Transaction) {
	sorter := byCreatedTimeAndPriority(transactions)
	if s.HighPriorityFirst {
		sort.Sort(sorter)
	} else {
		sort.Sort(sort.Reverse(sorter))
	}
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

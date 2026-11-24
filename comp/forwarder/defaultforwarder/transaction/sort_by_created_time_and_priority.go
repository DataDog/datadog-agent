// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transaction

import (
	"cmp"
	"slices"
)

// SortByCreatedTimeAndPriority sorts transactions by priority (highest first) and,
// for transactions with equal priority, by creation time (newest first).
func SortByCreatedTimeAndPriority(transactions []Transaction) {
	slices.SortFunc(transactions, func(a, b Transaction) int {
		return cmp.Or(
			cmp.Compare(b.GetPriority(), a.GetPriority()),
			b.GetCreatedAt().Compare(a.GetCreatedAt()),
		)
	})
}

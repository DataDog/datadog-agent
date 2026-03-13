// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package topn defines business logic for filtering NetFlow records to the Top "N" occurrences.
package topn

import (
	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

type sortFunc[T any] func(a, b T) int

func compareByBytesAscending(a, b *common.Flow) int {
	var aBytes uint64
	var bBytes uint64
	if a != nil {
		aBytes = a.Bytes
	}
	if b != nil {
		bBytes = b.Bytes
	}

	return int(aBytes - bBytes)
}

func reversed[T any](sortFunc sortFunc[T]) sortFunc[T] {
	return func(a, b T) int {
		return -sortFunc(a, b)
	}
}

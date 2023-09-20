// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package common

import "golang.org/x/exp/constraints"

// Min returns the smaller of two items, for any ordered type.
func Min[T constraints.Ordered](a T, b T) T {
	if a < b {
		return a
	}
	return b
}

// Max returns the larger of two items, for any ordered type.
func Max[T constraints.Ordered](a T, b T) T {
	if a > b {
		return a
	}
	return b
}

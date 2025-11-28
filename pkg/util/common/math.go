// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"math"
)

// ToPowerOf2 converts a number to its nearest power of 2
func ToPowerOf2(x int) int {
	log := math.Log2(float64(x))
	return int(math.Pow(2, math.Round(log)))
}

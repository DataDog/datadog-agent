// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package main

import (
	"fmt"
	"slices"
)

type iterExample struct{}

func (iterExample) rangeOverIterator() {
	for value := range slices.Values([]int{10, 20}) {
		fmt.Printf("Value: %d\n", value)
	}
}

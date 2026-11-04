// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"bytes"
	"slices"
)

// SSBytes supports sorting and searching for the [][]byte type
type SSBytes [][]byte

// Search returns the index of element x if found or -1 otherwise.
// SSBytes is expected to be sorted.
func (ss SSBytes) Search(x []byte) int {
	if i, found := slices.BinarySearchFunc(ss, x, bytes.Compare); found {
		return i
	}

	return -1
}

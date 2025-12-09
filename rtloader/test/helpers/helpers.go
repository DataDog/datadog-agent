// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helpers

import "sync/atomic"

var (
	// Allocations tracks number of memory allocations
	Allocations atomic.Uint64
	// Frees tracks number of memory frees
	Frees atomic.Uint64
)

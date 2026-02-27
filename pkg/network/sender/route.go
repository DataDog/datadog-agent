// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/indexedset"
)

const maxRoutes = math.MaxInt32

func formatRouteIndex(v *network.Via, routeSet *indexedset.IndexedSet[network.Via]) int32 {
	if v == nil || routeSet == nil || routeSet.Size() == maxRoutes {
		return -1
	}
	return routeSet.Add(*v)
}

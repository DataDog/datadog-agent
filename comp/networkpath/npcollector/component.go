// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package npcollector used to manage network paths
package npcollector

import (
	"iter"

	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
)

// team: network-path

// Component is the component type.
type Component interface {
	ScheduleNetworkPathTests(conns iter.Seq[npmodel.NetworkPathConnection])
}

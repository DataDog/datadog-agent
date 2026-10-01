// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides the mock for the npcollector component.
package mock

import (
	"iter"
	"testing"

	npcollector "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/def"
	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
)

// Mock implements the npcollector component with no-op methods.
type Mock struct{}

// ScheduleNetworkPathTests implements npcollector.Component.
func (*Mock) ScheduleNetworkPathTests(_ iter.Seq[npmodel.NetworkPathConnection]) {}

// ScheduleNetflowPathTests implements npcollector.Component.
func (*Mock) ScheduleNetflowPathTests(_ iter.Seq[npmodel.NetworkPathConnection]) {}

// New creates a mock npcollector component.
func New(_ testing.TB) npcollector.Component {
	return &Mock{}
}

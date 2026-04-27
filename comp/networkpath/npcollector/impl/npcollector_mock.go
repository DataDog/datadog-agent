// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"iter"

	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
)

type npCollectorMock struct{}

func (s *npCollectorMock) ScheduleNetworkPathTests(_conns iter.Seq[npmodel.NetworkPathConnection]) {}

// NewMock creates a mock npcollector component.
func NewMock() Provides {
	return Provides{
		Comp: &npCollectorMock{},
	}
}

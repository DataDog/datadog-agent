// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"iter"

	"go.uber.org/fx"

	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMock),
	)
}

type npCollectorMock struct{}

func (s *npCollectorMock) ScheduleNetworkPathTests(_conns iter.Seq[npmodel.NetworkPathConnection]) {}

func NewMock() Provides {
	// Mock initialization
	return Provides{
		Comp: &npCollectorMock{},
	}
}

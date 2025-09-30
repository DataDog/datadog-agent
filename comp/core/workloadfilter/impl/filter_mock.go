// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package workloadfilterimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadfiltermock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/mock"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MockRequires is a struct containing the required components for the mock.
type MockRequires struct {
	Config    config.Component
	Log       logdef.Component
	Telemetry coretelemetry.Component
}

// MockProvides is a struct containing the mock.
type MockProvides struct {
	Comp workloadfiltermock.Mock
}

// NewMock instantiates a new fakeTagger.
func NewMock(req MockRequires) MockProvides {
	filter, err := newFilter(req.Config, req.Log, req.Telemetry)
	if err != nil {
		log.Errorf("Failed to create filter component: %v", err)
	}

	return MockProvides{
		Comp: filter,
	}
}

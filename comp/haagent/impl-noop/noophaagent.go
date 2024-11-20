// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package haagentimpl provides a noop haagent component
package haagentimpl

import (
	"go.uber.org/fx"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type haagentimpl struct {
	Logger log.Component
}

func (m *haagentimpl) GetGroup() string { return "" }

func (m *haagentimpl) Enabled() bool { return false }

func (m *haagentimpl) SetLeader(_ string) {}

func (m *haagentimpl) IsLeader() bool { return false }

// NewNoopHaAgent returns a new Mock
func NewNoopHaAgent() haagent.Component {
	return &haagentimpl{}
}

// NoopModule defines the fx options for the haagentimpl component.
func NoopModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewNoopHaAgent),
	)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"go.uber.org/fx"

	model "github.com/DataDog/agent-payload/v5/process"
)

// Payload defines payload from the check
type Payload struct {
	CheckName string
	Message   []model.MessageBody
}

// Check defines an interface implemented by checks
type Check interface {
	checks.Check
}

// ProvidesCheck wraps a check implementation for consumption in components
type ProvidesCheck struct {
	fx.Out

	Check Check `group:"check"`
}

type RTResponse []*model.CollectorStatus

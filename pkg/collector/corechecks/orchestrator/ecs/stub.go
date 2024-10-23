// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !orchestrator

// Package ecs is used for the orchestrator ECS check
package ecs

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	// CheckName is the name of the check
	CheckName = "orchestrator_ecs"
)

// Factory creates a new check factory
func Factory(workloadmeta.Component, tagger.Component) optional.Option[func() check.Check] {
	return optional.NewNoneOption[func() check.Check]()
}

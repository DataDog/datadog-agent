// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !kubeapiserver

// Package schedulerevents implements the check that emits the events of the
// batch schedulers (Kueue and KAI Scheduler) from the Cluster Agent.
package schedulerevents

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "job_scheduler_events"
)

// Factory creates a new check factory
func Factory(_ workloadmeta.Component, _ tagger.Component, _ config.Component) option.Option[func() check.Check] {
	return option.None[func() check.Check]()
}

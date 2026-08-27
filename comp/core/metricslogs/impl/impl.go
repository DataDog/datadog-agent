// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metricslogsimpl contains the implementation of the metricslogs component.
package metricslogsimpl

import (
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	metricslogs "github.com/DataDog/datadog-agent/comp/core/metricslogs/def"
)

// Requires defines the dependencies for the metricslogs component.
type Requires struct {
	Log logcomp.Component
}

// Provides defines the output of the metricslogs component.
type Provides struct {
	Comp metricslogs.Component
}

type metricsLogsComponent struct {
	log logcomp.Component
}

// NewComponent creates a new metricslogs component.
func NewComponent(reqs Requires) Provides {
	return Provides{Comp: &metricsLogsComponent{log: reqs.Log}}
}

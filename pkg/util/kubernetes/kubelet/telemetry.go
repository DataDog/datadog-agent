// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

const (
	subsystem = "kubelet"
)

var (
	// queries tracks kubelet queries done by the Agent.
	queries = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		subsystem,
		"queries",
		[]string{"path", "code"},
		"Count of kubelet queries by path and response code. The response code defaults to 0 for unachieved queries. (The metric doesn't include kubelet check queries).",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
)

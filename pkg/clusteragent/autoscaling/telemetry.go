// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	workqueuetelemetry "github.com/DataDog/datadog-agent/pkg/util/workqueue/telemetry"
)

const (
	subsystem = "autoscaling"
)

var commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}

var (
	queueMetricsProvider = workqueuetelemetry.NewQueueMetricsProvider()
)

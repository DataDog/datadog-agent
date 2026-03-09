// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	workqueuetelemetry "github.com/DataDog/datadog-agent/pkg/util/workqueue/telemetry"
)

const (
	subsystem = "autoscaling_cluster"
)

var autoscalingQueueMetricsProvider = workqueuetelemetry.NewQueueMetricsProvider()

// TODO add more telemetry collection

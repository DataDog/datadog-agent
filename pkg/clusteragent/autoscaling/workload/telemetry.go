// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	subsystem = "workload_autoscaling"
)

var commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}

func resourceListToString(m map[corev1.ResourceName]resource.Quantity) string {
	var sb strings.Builder
	for k, v := range m {
		sb.WriteString(fmt.Sprintf("%s:%s", string(k), v.String()))
	}
	return sb.String()
}

var (
	// rolloutTriggered tracks the number of patch requests sent by the patcher to the kubernetes api server
	rolloutTriggered = telemetry.NewCounterWithOpts(
		subsystem,
		"rollout_triggered",
		[]string{"owner_kind", "owner_name", "namespace", "status"},
		"Tracks the number of patch requests sent by the patcher to the kubernetes api server",
		commonOpts,
	)

	// telemetryHorizonalScale tracks the horizontal scaling recommendation values
	telemetryHorizonalScale = telemetry.NewCounterWithOpts(
		subsystem,
		"horizontal_scaling",
		[]string{"namespace", "resource_name", "from_replicas", "to_replicas", "recommended_replicas", "is_error"},
		"Tracks the number of horizontal scaling events triggered",
		commonOpts,
	)

	// telemetryVerticalScale tracks the vertical scaling recommendation values
	telemetryVerticalScale = telemetry.NewCounterWithOpts(
		subsystem,
		"vertical_scaling",
		[]string{"namespace", "name", "source", "container_name", "limits", "requests", "is_error"},
		"Tracks the number of vertical scaling events triggered",
		commonOpts,
	)
)

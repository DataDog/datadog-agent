// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package common contains shared stores and targeting helpers for instrumentation components.
package common

import (
	"fmt"

	autoscalingv2 "k8s.io/api/autoscaling/v2"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

// AutodiscoveryProvider identifies configurations produced by DatadogInstrumentation.
const AutodiscoveryProvider = "datadoginstrumentation"

// RootOwnerCELFilter selects containers owned by the targeted workload in its namespace.
func RootOwnerCELFilter(ref autoscalingv2.CrossVersionObjectReference, namespace string) workloadfilter.Rules {
	expr := fmt.Sprintf(
		`container.pod.rootowner.kind == %q && container.pod.rootowner.name == %q && container.pod.namespace == %q && container.image.reference != ""`,
		ref.Kind, ref.Name, namespace,
	)
	return workloadfilter.Rules{
		Containers: []string{expr},
	}
}

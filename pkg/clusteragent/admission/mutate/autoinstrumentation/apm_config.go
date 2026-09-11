// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import "github.com/DataDog/datadog-agent/pkg/ssi"

// DDITargetProvider surfaces a SSI configuration for pods part of a workload targeted
// by a DatadogInstrumentation custom resource in the cluster.
type DDITargetProvider interface {
	GetTarget(ssi.WorkloadTarget) (ssi.DDITarget, bool)
}

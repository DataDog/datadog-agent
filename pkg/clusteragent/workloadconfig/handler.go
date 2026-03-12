// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workloadconfig

import datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

// ConfigSectionHandler processes one section of a DatadogInstrumentation CR.
// Handlers receive the full list of CRs each cycle (full-sync model).
// Errors in one handler do not block others.
type ConfigSectionHandler interface {
	Name() string
	Reconcile(crs []*datadoghq.DatadogInstrumentation) error
}

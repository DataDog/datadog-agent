// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package logs defines the DatadogInstrumentation logs handler component.
package logs

import "github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"

// team: container-platform

// Component validates and reconciles the logs section of a DatadogInstrumentation.
// Its methods may be called concurrently by the controller and admission webhook.
type Component interface {
	instrumentation.Handler
}

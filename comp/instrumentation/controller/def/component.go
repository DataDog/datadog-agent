// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package controller defines the DatadogInstrumentation reconciliation component.
package controller

import (
	"context"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
)

// team: container-platform

// Component dispatches DatadogInstrumentation events to product handlers.
type Component interface {
	// Start registers the informer and starts reconciliation in the background.
	// Call it once, after the CRD is available, before starting the informer factory.
	// The caller starts the informer factory and cancels ctx to stop reconciliation.
	Start(ctx context.Context, params Params) error
}

// Params binds the controller to the initialized Kubernetes clients and leadership state.
type Params struct {
	StatusClient    dynamic.Interface
	InformerFactory dynamicinformer.DynamicSharedInformerFactory
	IsLeader        func() bool
}

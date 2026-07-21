// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !kubeapiserver

// Package helmactionsimpl implements the helmactions component interface.
package helmactionsimpl

import (
	batchv1 "k8s.io/api/batch/v1"

	helmactions "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/def"
)

// Requires defines the dependencies for the helmactions component.
type Requires struct {
}

// Provides defines the output of the helmactions component.
type Provides struct {
	Comp helmactions.Component
}

type helmactionsImpl struct {
}

// OnRollback is a no-op on platforms built without kubeapiserver support.
func (h *helmactionsImpl) OnRollback(in *helmactions.RollbackInputs, job *batchv1.Job) {
}

// NewComponent creates a new helmactions component.
func NewComponent(reqs Requires) (Provides, error) {
	comp := &helmactionsImpl{}

	return Provides{Comp: comp}, nil
}

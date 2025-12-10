// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_apps

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RollbackDeploymentHandler struct {
}

func NewRollbackDeploymentHandler() *RollbackDeploymentHandler {
	return &RollbackDeploymentHandler{}
}

type RollbackDeploymentInputs struct {
	*support.GetFields
	Namespace      string `json:"namespace,omitempty"`
	ToRevision     int64  `json:"toRevision,omitempty"`
	DryRunStrategy string `json:"dryRunStrategy,omitempty"`
}

type RollbackDeploymentOutputs struct{}

func (h *RollbackDeploymentHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (interface{}, error) {
	//TODO
	return &RollbackDeploymentOutputs{}, nil
}

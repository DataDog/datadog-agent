// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_rbac

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteClusterRoleHandler struct{}

func NewDeleteClusterRoleHandler() *DeleteClusterRoleHandler {
	return &DeleteClusterRoleHandler{}
}

type DeleteClusterRoleInputs struct {
	*support.DeleteFields
}

type DeleteClusterRoleOutputs struct{}

func (h *DeleteClusterRoleHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[DeleteClusterRoleInputs](task)
	if err != nil {
		return nil, err
	}

	if inputs.DeleteFields == nil || inputs.Name == "" {
		return nil, errors.New("name is required")
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	err = client.RbacV1().ClusterRoles().Delete(ctx, inputs.Name, support.MetaDelete(inputs.DeleteFields))
	if err != nil {
		return nil, err
	}

	return &DeleteClusterRoleOutputs{}, nil
}

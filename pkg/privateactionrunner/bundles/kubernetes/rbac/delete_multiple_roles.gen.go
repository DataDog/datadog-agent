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

type DeleteMultipleRolesHandler struct{}

func NewDeleteMultipleRolesHandler() *DeleteMultipleRolesHandler {
	return &DeleteMultipleRolesHandler{}
}

type DeleteMultipleRolesInputs struct {
	*support.DeleteFields
	*support.ListFields
	Namespace string `json:"namespace,omitempty"`
}

type DeleteMultipleRolesOutputs struct{}

func (h *DeleteMultipleRolesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[DeleteMultipleRolesInputs](task)
	if err != nil {
		return nil, err
	}

	if inputs.Namespace == "" {
		return nil, errors.New("namespace is required")
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	err = client.RbacV1().Roles(inputs.Namespace).DeleteCollection(ctx, support.MetaDelete(inputs.DeleteFields), support.MetaList(inputs.ListFields))
	if err != nil {
		return nil, err
	}

	return &DeleteMultipleRolesOutputs{}, nil
}

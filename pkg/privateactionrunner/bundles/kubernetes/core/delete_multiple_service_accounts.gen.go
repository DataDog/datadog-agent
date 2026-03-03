// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_core

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteMultipleServiceAccountsHandler struct{}

func NewDeleteMultipleServiceAccountsHandler() *DeleteMultipleServiceAccountsHandler {
	return &DeleteMultipleServiceAccountsHandler{}
}

type DeleteMultipleServiceAccountsInputs struct {
	*support.DeleteFields
	*support.ListFields
	Namespace string `json:"namespace,omitempty"`
}

type DeleteMultipleServiceAccountsOutputs struct{}

func (h *DeleteMultipleServiceAccountsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[DeleteMultipleServiceAccountsInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	err = client.CoreV1().ServiceAccounts(inputs.Namespace).DeleteCollection(ctx, support.MetaDelete(inputs.DeleteFields), support.MetaList(inputs.ListFields))
	if err != nil {
		return nil, err
	}

	return &DeleteMultipleServiceAccountsOutputs{}, nil
}

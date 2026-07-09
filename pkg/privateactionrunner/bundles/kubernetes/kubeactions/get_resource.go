// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package com_datadoghq_kubernetes_kubeactions

import (
	"context"
	"encoding/json"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
	kubeactionsimpl "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/impl"
	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// GetResourceHandler handles the get_resource action.
type GetResourceHandler struct {
	ka kubeactions.Component
}

// NewGetResourceHandler creates a new GetResourceHandler.
func NewGetResourceHandler(ka kubeactions.Component) types.Action {
	return &GetResourceHandler{ka: ka}
}

// GetResourceOutputs is the result returned to the Datadog backend for the
// get_resource action. Resources maps "<kind>/<namespace>/<name>" to the
// scrubbed resource JSON.
type GetResourceOutputs struct {
	Status    string                     `json:"status"`
	Message   string                     `json:"message"`
	Resources map[string]json.RawMessage `json:"resources,omitempty"`
}

// Run executes the get_resource action.
func (h *GetResourceHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	in, err := types.ExtractInputs[kubeactions.GetResourceInputs](task)
	if err != nil {
		return nil, err
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}

	client, err := support.DynamicKubeClient(credential)
	if err != nil {
		return nil, err
	}

	report := newReport(kubeactions.ActionTypeGetResource, in.ResourceRef, task)
	h.ka.ReportReceived(report)

	result := kubeactionsimpl.NewGetResourceExecutor(client).Execute(ctx, in)
	h.ka.ReportResult(report, result)

	out := &GetResourceOutputs{Status: result.Status, Message: result.Message}
	if len(result.Payloads) > 0 {
		out.Resources = make(map[string]json.RawMessage, len(result.Payloads))
		for k, v := range result.Payloads {
			out.Resources[k] = json.RawMessage(v)
		}
	}
	return out, nil
}

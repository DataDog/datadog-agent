// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_apps

import (
	"context"
	"fmt"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/polymorphichelpers"
)

type RollbackDeploymentHandler struct {
	c kubernetes.Interface
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
	credential interface{},
) (interface{}, error) {
	inputs, err := types.ExtractInputs[RollbackDeploymentInputs](task)
	if err != nil {
		return nil, err
	}
	client, err := h.getClient(credential)
	if err != nil {
		return nil, err
	}

	deployment, err := client.AppsV1().Deployments(inputs.Namespace).Get(ctx, inputs.Name, support.MetaGet(inputs.GetFields))
	if err != nil {
		return "", fmt.Errorf("failed to retrieve Deployment %s: %v", inputs.Name, err)
	}
	// Ideally we should use deployment.GroupVersionKind().GroupKind() but it can be empty
	kind := schema.GroupKind{Group: "apps", Kind: "Deployment"}
	rollbacker, err := polymorphichelpers.RollbackerFor(kind, client)
	if err != nil {
		return nil, err
	}
	dryRunStrategy, err := getDryRunStrategy(inputs.DryRunStrategy)
	if err != nil {
		return nil, err
	}
	_, err = rollbacker.Rollback(deployment, nil, inputs.ToRevision, dryRunStrategy)
	if err != nil {
		return nil, err
	}
	return &RollbackDeploymentOutputs{}, nil
}

func getDryRunStrategy(dryRunStrategy string) (cmdutil.DryRunStrategy, error) {
	switch dryRunStrategy {
	case "", "none":
		return cmdutil.DryRunNone, nil
	case "client":
		return cmdutil.DryRunClient, nil
	case "server":
		return cmdutil.DryRunServer, nil
	default:
		return cmdutil.DryRunNone, fmt.Errorf(`invalid dry-run value (%v). Must be "none", "server", or "client"`, dryRunStrategy)
	}
}

func (h *RollbackDeploymentHandler) getClient(credential interface{}) (kubernetes.Interface, error) {
	if h.c != nil {
		return h.c, nil
	}
	return support.KubeClient(credential)
}

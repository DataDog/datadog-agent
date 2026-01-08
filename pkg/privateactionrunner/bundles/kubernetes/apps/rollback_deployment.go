// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_apps

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

const RevisionAnnotation = "deployment.kubernetes.io/revision"

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
	credential *privateconnection.PrivateCredentials,
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
		return nil, fmt.Errorf("failed to retrieve Deployment %s: %v", inputs.Name, err)
	}

	if inputs.DryRunStrategy == "client" {
		return &RollbackDeploymentOutputs{}, nil
	}

	targetRS, err := h.getTargetReplicaSet(ctx, client, deployment, inputs.ToRevision)
	if err != nil {
		return nil, err
	}

	deployment.Spec.Template = targetRS.Spec.Template

	updateOpts := metav1.UpdateOptions{}
	if inputs.DryRunStrategy == "server" {
		updateOpts.DryRun = []string{metav1.DryRunAll}
	}

	_, err = client.AppsV1().Deployments(inputs.Namespace).Update(ctx, deployment, updateOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to rollback Deployment %s: %v", inputs.Name, err)
	}

	return &RollbackDeploymentOutputs{}, nil
}

func (h *RollbackDeploymentHandler) getTargetReplicaSet(
	ctx context.Context,
	client kubernetes.Interface,
	deployment *appsv1.Deployment,
	toRevision int64,
) (*appsv1.ReplicaSet, error) {
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("failed to parse deployment selector: %v", err)
	}

	allRS, err := client.AppsV1().ReplicaSets(deployment.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list ReplicaSets: %v", err)
	}

	// Filter to ReplicaSets owned by this deployment with valid revision annotations
	var ownedRS []*appsv1.ReplicaSet
	for i := range allRS.Items {
		rs := &allRS.Items[i]
		if !metav1.IsControlledBy(rs, deployment) {
			continue
		}
		if _, ok := rs.Annotations[RevisionAnnotation]; !ok {
			continue
		}
		ownedRS = append(ownedRS, rs)
	}

	if len(ownedRS) == 0 {
		return nil, fmt.Errorf("no valid ReplicaSets found for deployment %s", deployment.Name)
	}

	// Sort by revision number (descending) - required, if the revision is not specified
	sort.Slice(ownedRS, func(i, j int) bool {
		revI, _ := strconv.ParseInt(ownedRS[i].Annotations[RevisionAnnotation], 10, 64)
		revJ, _ := strconv.ParseInt(ownedRS[j].Annotations[RevisionAnnotation], 10, 64)
		return revI > revJ
	})

	if toRevision == 0 {
		if len(ownedRS) < 2 {
			return nil, fmt.Errorf("no previous revision found for deployment %s", deployment.Name)
		}
		return ownedRS[1], nil
	}

	for _, rs := range ownedRS {
		rev, err := strconv.ParseInt(rs.Annotations[RevisionAnnotation], 10, 64)
		if err != nil {
			continue
		}
		if rev == toRevision {
			return rs, nil
		}
	}

	return nil, fmt.Errorf("revision %d not found for deployment %s", toRevision, deployment.Name)
}

func (h *RollbackDeploymentHandler) getClient(credential *privateconnection.PrivateCredentials) (kubernetes.Interface, error) {
	if h.c != nil {
		return h.c, nil
	}
	return support.KubeClient(credential)
}

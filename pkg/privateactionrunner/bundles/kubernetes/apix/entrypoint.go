// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package com_datadoghq_kubernetes_apix

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type KubernetesApiExtensions struct {
}

func NewKubernetesApiExtensions() *KubernetesApiExtensions {
	return &KubernetesApiExtensions{}
}

func (b *KubernetesApiExtensions) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "createCustomResourceDefinition":
		return b.RunCreateCustomResourceDefinition(ctx, task, credential)
	case "deleteCustomResourceDefinition":
		return b.RunDeleteCustomResourceDefinition(ctx, task, credential)
	case "deleteMultipleCustomResourceDefinitions":
		return b.RunDeleteMultipleCustomResourceDefinitions(ctx, task, credential)
	case "getCustomResourceDefinition":
		return b.RunGetCustomResourceDefinition(ctx, task, credential)
	case "listCustomResourceDefinition":
		return b.RunListCustomResourceDefinition(ctx, task, credential)
	case "patchCustomResourceDefinition":
		return b.RunPatchCustomResourceDefinition(ctx, task, credential)
	case "updateCustomResourceDefinition":
		return b.RunUpdateCustomResourceDefinition(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (b *KubernetesApiExtensions) GetAction(_ string) types.Action {
	return b
}

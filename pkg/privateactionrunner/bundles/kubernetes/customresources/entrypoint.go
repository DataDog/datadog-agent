// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package com_datadoghq_kubernetes_customresources

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type KubernetesCustomResources struct {
}

func NewKubernetesCustomResources() *KubernetesCustomResources {
	return &KubernetesCustomResources{}
}

func (b *KubernetesCustomResources) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "createCustomObject":
		return b.RunCreateCustomObject(ctx, task, credential)
	case "deleteCustomObject":
		return b.RunDeleteCustomObject(ctx, task, credential)
	case "deleteMultipleCustomObjects":
		return b.RunDeleteMultipleCustomObjects(ctx, task, credential)
	case "getCustomObject":
		return b.RunGetCustomObject(ctx, task, credential)
	case "listCustomObject":
		return b.RunListCustomObject(ctx, task, credential)
	case "patchCustomObject":
		return b.RunPatchCustomObject(ctx, task, credential)
	case "updateCustomObject":
		return b.RunUpdateCustomObject(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (b *KubernetesCustomResources) GetAction(_ string) types.Action {
	return b
}

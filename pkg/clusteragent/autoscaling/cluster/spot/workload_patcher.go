// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

// workloadPatcher patches Kubernetes workload annotations.
type workloadPatcher interface {
	// setDisabledUntil patches the SpotDisabledUntilAnnotation on the workload.
	setDisabledUntil(ctx context.Context, owner workload, until time.Time) error
}

type kubeWorkloadPatcher struct {
	dynamicClient dynamic.Interface
}

func newKubeWorkloadPatcher(dynamicClient dynamic.Interface) *kubeWorkloadPatcher {
	return &kubeWorkloadPatcher{dynamicClient: dynamicClient}
}

func (p *kubeWorkloadPatcher) setDisabledUntil(ctx context.Context, owner workload, until time.Time) error {
	var gvrVal *workloadResource
	for i := range spotWorkloadResources {
		if spotWorkloadResources[i].kind == owner.Kind {
			gvrVal = &spotWorkloadResources[i]
			break
		}
	}
	if gvrVal == nil {
		return fmt.Errorf("unknown workload kind %q", owner.Kind)
	}

	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]string{
				SpotDisabledUntilAnnotation: until.UTC().Format(time.RFC3339),
			},
		},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	_, err = p.dynamicClient.Resource(gvrVal.gvr).Namespace(owner.Namespace).Patch(
		ctx, owner.Name, types.MergePatchType, data, metav1.PatchOptions{},
	)
	return err
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"fmt"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	scaleclient "k8s.io/client-go/scale"
)

type scaler interface {
	get(ctx context.Context, namespace, name string, gvk schema.GroupVersionKind) (*autoscalingv1.Scale, schema.GroupResource, error)
	update(ctx context.Context, gr schema.GroupResource, scale *autoscalingv1.Scale) (*autoscalingv1.Scale, error)
}

type scalerImpl struct {
	restMapper  apimeta.RESTMapper
	scaleGetter scaleclient.ScalesGetter
}

func newScaler(restMapper apimeta.RESTMapper, scaleGetter scaleclient.ScalesGetter) scaler {
	return &scalerImpl{
		restMapper:  restMapper,
		scaleGetter: scaleGetter,
	}
}

func (sg *scalerImpl) get(ctx context.Context, namespace, name string, gvk schema.GroupVersionKind) (*autoscalingv1.Scale, schema.GroupResource, error) {
	mappings, err := sg.restMapper.RESTMappings(gvk.GroupKind())
	if err != nil {
		return nil, schema.GroupResource{}, fmt.Errorf("failed to get REST mappings for GVK: %s", gvk)
	}

	var firstErr error
	for i, mapping := range mappings {
		targetGR := mapping.Resource.GroupResource()
		scale, err := sg.scaleGetter.Scales(namespace).Get(ctx, targetGR, name, metav1.GetOptions{})
		if err == nil {
			return scale, targetGR, nil
		}

		// if this is the first error, remember it,
		// then go on and try other mappings until we find a good one
		if i == 0 {
			firstErr = err
		}
	}

	// make sure we handle an empty set of mappings
	if firstErr == nil {
		firstErr = fmt.Errorf("unrecognized resource: %s", gvk)
	}

	return nil, schema.GroupResource{}, firstErr
}

func (sg *scalerImpl) update(ctx context.Context, gr schema.GroupResource, scale *autoscalingv1.Scale) (*autoscalingv1.Scale, error) {
	return sg.scaleGetter.Scales(scale.Namespace).Update(ctx, gr, scale, metav1.UpdateOptions{})
}

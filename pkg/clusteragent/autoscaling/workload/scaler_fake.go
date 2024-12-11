// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package workload

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/stretchr/testify/mock"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type fakeScaler struct {
	mock.Mock
}

func newFakeScaler() *fakeScaler {
	return &fakeScaler{}
}

func (fs *fakeScaler) get(ctx context.Context, namespace, name string, gvk schema.GroupVersionKind) (*autoscalingv1.Scale, schema.GroupResource, error) {
	args := fs.Called(ctx, namespace, name, gvk)
	return args.Get(0).(*autoscalingv1.Scale), args.Get(1).(schema.GroupResource), args.Error(2)
}

func (fs *fakeScaler) update(ctx context.Context, gr schema.GroupResource, scale *autoscalingv1.Scale) (*autoscalingv1.Scale, error) {
	args := fs.Called(ctx, gr, scale)
	return args.Get(0).(*autoscalingv1.Scale), args.Error(1)
}

func (fs *fakeScaler) mockGet(pai model.FakePodAutoscalerInternal, specReplicas, statusReplicas int32, err error) {
	mockCall := fs.On("get", mock.Anything, pai.Namespace, pai.Name, pai.TargetGVK)
	if err != nil {
		mockCall.Return(nil, schema.GroupResource{}, err)
		return
	}

	mockCall.Return(
		&autoscalingv1.Scale{
			ObjectMeta: v1.ObjectMeta{
				Namespace: pai.Namespace,
				Name:      pai.Spec.TargetRef.Name,
			},
			Spec: autoscalingv1.ScaleSpec{
				Replicas: specReplicas,
			},
			Status: autoscalingv1.ScaleStatus{
				Replicas: statusReplicas,
			},
		},
		schema.GroupResource{Group: pai.TargetGVK.Group, Resource: pai.TargetGVK.Kind},
		nil,
	)
}

func (fs *fakeScaler) mockUpdate(pai model.FakePodAutoscalerInternal, specReplicas, statusReplicas int32, err error) {
	mockCall := fs.On("update",
		mock.Anything,
		schema.GroupResource{Group: pai.TargetGVK.Group, Resource: pai.TargetGVK.Kind},
		&autoscalingv1.Scale{
			ObjectMeta: v1.ObjectMeta{
				Namespace: pai.Namespace,
				Name:      pai.Spec.TargetRef.Name,
			},
			Spec: autoscalingv1.ScaleSpec{
				Replicas: specReplicas,
			},
			Status: autoscalingv1.ScaleStatus{
				Replicas: statusReplicas,
			},
		},
	)

	if err != nil {
		var nilScale *autoscalingv1.Scale
		mockCall.Return(nilScale, err)
		return
	}
	mockCall.Return(&autoscalingv1.Scale{
		ObjectMeta: v1.ObjectMeta{
			Namespace: pai.Namespace,
			Name:      pai.Spec.TargetRef.Name,
		},
		Spec: autoscalingv1.ScaleSpec{
			Replicas: specReplicas,
		},
		Status: autoscalingv1.ScaleStatus{
			Replicas: statusReplicas,
		},
	}, nil)
}

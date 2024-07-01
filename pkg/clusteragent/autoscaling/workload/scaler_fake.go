// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package workload

import (
	"context"

	"github.com/stretchr/testify/mock"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type fakeScaler struct {
	mock.Mock
}

func newFakeScaler() scaler {
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

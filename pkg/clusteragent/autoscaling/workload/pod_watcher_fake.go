// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package workload

import (
	"context"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/stretchr/testify/mock"
)

type fakePodWatcher struct {
	mock.Mock
}

func newFakePodWatcher() *fakePodWatcher {
	return &fakePodWatcher{}
}

func (f *fakePodWatcher) Run(ctx context.Context) {
	f.Called(ctx)
}

func (f *fakePodWatcher) GetPodsForOwner(owner NamespacedPodOwner) []*workloadmeta.KubernetesPod {
	return f.Called(owner).Get(0).([]*workloadmeta.KubernetesPod)
}

func (f *fakePodWatcher) GetReadyPodsForOwner(owner NamespacedPodOwner) int32 {
	return f.Called(owner).Get(0).(int32)
}

func (f *fakePodWatcher) mockGetPodsForOwner(owner NamespacedPodOwner, pods []*workloadmeta.KubernetesPod) {
	mockCall := f.On("GetPodsForOwner", owner)
	mockCall.Return(pods)
}

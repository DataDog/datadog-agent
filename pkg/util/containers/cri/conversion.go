// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build cri

/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cri

import (
	"unsafe"

	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func fromV1alpha2VersionResponse(from *v1alpha2.VersionResponse) *runtimeapi.VersionResponse {
	return (*runtimeapi.VersionResponse)(unsafe.Pointer(from))
}

func fromV1alpha2ListContainerStatsResponse(from *v1alpha2.ListContainerStatsResponse) *runtimeapi.ListContainerStatsResponse {
	return (*runtimeapi.ListContainerStatsResponse)(unsafe.Pointer(from))
}

func v1alpha2ContainerStatsFilter(from *runtimeapi.ContainerStatsFilter) *v1alpha2.ContainerStatsFilter {
	return (*v1alpha2.ContainerStatsFilter)(unsafe.Pointer(from))
}

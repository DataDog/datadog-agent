// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filterpb contains helpers for proto definitions.
package filterpb

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func CreateFilterPodContainer(container workloadmeta.Container, pod workloadmeta.KubernetesPod) *pbgo.FilterContainer {
	return &pbgo.FilterContainer{
		Id:    container.ID,
		Name:  container.Name,
		Image: container.Image.Name,
		Owner: &pbgo.FilterContainer_Pod{
			Pod: &pbgo.FilterPod{
				Id:          pod.ID,
				Name:        pod.Name,
				Namespace:   pod.Namespace,
				Annotations: pod.Annotations,
			},
		},
	}
}

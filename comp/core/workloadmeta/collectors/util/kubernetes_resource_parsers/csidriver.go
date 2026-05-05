// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	storagev1 "k8s.io/api/storage/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

type csiDriverParser struct{}

// NewCSIDriverParser initialises and returns a parser for storage.k8s.io/v1.CSIDriver.
func NewCSIDriverParser() ObjectParser {
	return csiDriverParser{}
}

func (p csiDriverParser) Parse(obj interface{}) workloadmeta.Entity {
	driver := obj.(*storagev1.CSIDriver)

	modes := make([]string, 0, len(driver.Spec.VolumeLifecycleModes))
	for _, m := range driver.Spec.VolumeLifecycleModes {
		modes = append(modes, string(m))
	}

	return &workloadmeta.KubernetesCSIDriver{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesCSIDriver,
			ID:   driver.Name,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        driver.Name,
			Labels:      driver.Labels,
			Annotations: driver.Annotations,
		},
		Spec: workloadmeta.KubernetesCSIDriverSpec{
			VolumeLifecycleModes: modes,
		},
	}
}

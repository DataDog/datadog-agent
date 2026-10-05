// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package benchmarks

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
)

func runPrimitive(raw []byte, scenario string) ([]byte, error) {
	return common.MutateWithPatch(raw, "benchmark", "benchmark", func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		switch scenario {
		case "annotation":
			return false, s.SetAnnotations(map[string]string{"example.com/test": "yes"}, false)
		case "resources":
			pod, err := s.Snapshot()
			if err != nil {
				return false, err
			}
			q := resource.MustParse("100m")
			var edits []patch.ResourceEdit
			for _, c := range pod.Spec.Containers {
				edits = append(edits, patch.ResourceEdit{Container: patch.ContainerID{Kind: patch.RegularContainers, Name: c.Name}, Name: corev1.ResourceCPU, Quantity: &q})
			}
			return true, s.EditResources(edits)
		case "toleration":
			return true, s.AppendToleration(corev1.Toleration{Key: "spot", Operator: corev1.TolerationOpExists})
		default:
			return false, nil
		}
	}, nil)
}

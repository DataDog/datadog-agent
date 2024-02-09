// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"reflect"
	"testing"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/stretchr/testify/assert"
)

func TestGetNamespacedBaseOwnerReferen(t *testing.T) {

	tests := []struct {
		name     string
		input    pbgo.PodLanguageDetails
		expected NamespacedOwnerReference
	}{
		{
			name: "Case of replicaset",
			input: pbgo.PodLanguageDetails{
				Namespace:            "default",
				Name:                 "pod-a",
				ContainerDetails:     []*pbgo.ContainerLanguageDetails{},
				InitContainerDetails: []*pbgo.ContainerLanguageDetails{},
				Ownerref: &pbgo.KubeOwnerInfo{
					Id:   "dummyId-1",
					Kind: "ReplicaSet",
					Name: "dummyrs-1-2342347",
				},
			},
			expected: NewNamespacedOwnerReference("apps/v1", "Deployment", "dummyrs-1", "dummyId-1", "default"),
		},
		{
			name: "Case of statefulset in custom namespace",
			input: pbgo.PodLanguageDetails{
				Namespace:            "custom",
				Name:                 "pod-b",
				ContainerDetails:     []*pbgo.ContainerLanguageDetails{},
				InitContainerDetails: []*pbgo.ContainerLanguageDetails{},
				Ownerref: &pbgo.KubeOwnerInfo{
					Id:   "dummyId-2",
					Kind: "StatefulSet",
					Name: "dummy-statefulset-name",
				},
			},
			expected: NewNamespacedOwnerReference("apps/v1", "StatefulSet", "dummy-statefulset-name", "dummyId-2", "custom"),
		},
	}

	for i := range tests {
		t.Run(tests[i].name, func(t *testing.T) {
			actual := getNamespacedBaseOwnerReference(&tests[i].input)
			assert.True(t, reflect.DeepEqual(tests[i].expected, actual))
		})
	}

}

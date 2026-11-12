// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package kubernetesresourceparsers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestKAIPodGroupParserFixtures(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "kai", "podgroups.yaml"))
	require.NoError(t, err)
	jsonList, err := yaml.YAMLToJSON(raw)
	require.NoError(t, err)
	var list unstructured.UnstructuredList
	require.NoError(t, list.UnmarshalJSON(jsonList))
	require.NotEmpty(t, list.Items)

	podGroup := NewKAIPodGroupParser().Parse(&list.Items[0]).(*workloadmeta.KubernetesKAIPodGroup)
	assert.Equal(t, &workloadmeta.KubernetesKAIPodGroup{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesKAIPodGroup,
			ID:   "kai-test/pg-frac-a-efc71a63-bdb9-4f0b-b95a-7309efddf661",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "pg-frac-a-efc71a63-bdb9-4f0b-b95a-7309efddf661",
			Namespace: "kai-test",
			Labels:    map[string]string{"kai.scheduler/queue": "default-queue"},
			Annotations: map[string]string{
				"gpu-fraction":                     "0.5",
				"kai.scheduler/top-owner-metadata": "name: frac-a\nuid: efc71a63-bdb9-4f0b-b95a-7309efddf661\ngroup: \"\"\nversion: v1\nkind: Pod\n",
				"runai/shared-gpu-configmap":       "frac-a-jb6btbp-shared-gpu",
			},
			UID: "355f982a-d7b8-4ef4-ab6e-232bb97a6ad2",
		},
		QueueName: "default-queue",
	}, podGroup)
}

func TestKAIPodGroupParserInvalidQueue(t *testing.T) {
	tests := []struct {
		name string
		spec map[string]interface{}
	}{
		{name: "no queue", spec: map[string]interface{}{"minMember": int64(1)}},
		{name: "queue is not a string", spec: map[string]interface{}{"queue": int64(1)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			obj := map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":      "pg",
					"namespace": "ns",
				},
				"spec": test.spec,
			}

			podGroup := NewKAIPodGroupParser().Parse(&unstructured.Unstructured{Object: obj}).(*workloadmeta.KubernetesKAIPodGroup)
			assert.Equal(t, "ns/pg", podGroup.ID)
			assert.Empty(t, podGroup.QueueName)
		})
	}
}

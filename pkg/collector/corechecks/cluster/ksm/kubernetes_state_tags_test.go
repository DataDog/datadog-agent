// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
)

func TestHostnameAndTagsRoomForCheckTags(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	namespaceID := types.NewEntityID(types.KubernetesMetadata, string(util.GenerateKubeMetadataEntityID("", "namespaces", "", "default")))
	fakeTagger.SetTags(namespaceID, "workloadmeta-kubernetes_metadata", []string{"team:a", "env:prod", "tier:web"}, nil, nil, nil)

	k := newKSMCheck(core.NewCheckBase(CheckName), &KSMConfig{
		LabelsMapper: map[string]string{},
		Tags:         []string{"check:a", "check:b", "check:c"},
	}, fakeTagger, nil)
	labels, err := ksmstore.NewLabels(
		[]string{"namespace", "pod", "created_by_kind", "created_by_name"},
		[]string{"default", "app-6768ddc4d-x7k2p", "ReplicaSet", "app-6768ddc4d"},
	)
	require.NoError(t, err)

	_, tags := k.hostnameAndTags("kube_pod_info", labels, newLabelJoiner(nil), nil)
	assert.ElementsMatch(t, []string{
		"namespace:default",
		"pod:app-6768ddc4d-x7k2p",
		"kube_replica_set:app-6768ddc4d",
		"kube_deployment:app",
		"team:a",
		"env:prod",
		"tier:web",
	}, tags)
	// The sender appends the check tags in place
	assert.GreaterOrEqual(t, cap(tags)-len(tags), len(k.instance.Tags))

	// The tags of a series don't share the buffer they are built in
	first, want := tags, slices.Clone(tags)
	other, err := ksmstore.NewLabels([]string{"namespace", "pod"}, []string{"default", "other"})
	require.NoError(t, err)
	_, _ = k.hostnameAndTags("kube_pod_info", other, newLabelJoiner(nil), nil)
	assert.Equal(t, want, first)

	// The namespace tags are cached for the run
	fakeTagger.SetTags(namespaceID, "workloadmeta-kubernetes_metadata", []string{"team:b"}, nil, nil, nil)
	_, tags = k.hostnameAndTags("kube_pod_info", labels, newLabelJoiner(nil), nil)
	assert.Contains(t, tags, "team:a")
	k.resetRunCaches()
	_, tags = k.hostnameAndTags("kube_pod_info", labels, newLabelJoiner(nil), nil)
	assert.Contains(t, tags, "team:b")
	assert.NotContains(t, tags, "team:a")
}

func TestLabelJoinerReusesItsBuffer(t *testing.T) {
	lj := newLabelJoiner(map[string]*joinsConfig{
		"kube_pod_info": {
			labelsToMatch: []string{"namespace", "pod"},
			labelsToGet:   map[string]string{"node": "node"},
		},
	})
	info, err := ksmstore.NewLabels([]string{"namespace", "pod", "node"}, []string{"default", "pod-a", "node-1"})
	require.NoError(t, err)
	lj.insertFamily(ksmstore.DDMetricsFam{Name: "kube_pod_info", ListMetrics: []ksmstore.DDMetric{{Labels: info, Val: 1}}})

	series, err := ksmstore.NewLabels([]string{"namespace", "pod"}, []string{"default", "pod-a"})
	require.NoError(t, err)
	first := lj.getLabelsToAdd(series)
	require.Equal(t, []label{{key: "node", value: "node-1"}}, first)
	second := lj.getLabelsToAdd(series)
	require.Equal(t, []label{{key: "node", value: "node-1"}}, second)
	assert.Same(t, &first[0], &second[0])

	other, err := ksmstore.NewLabels([]string{"namespace", "pod"}, []string{"default", "pod-b"})
	require.NoError(t, err)
	assert.Empty(t, lj.getLabelsToAdd(other))
}

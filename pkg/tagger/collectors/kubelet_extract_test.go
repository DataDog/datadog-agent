// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package collectors

import (
	"encoding/json"
	"io/ioutil"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

func requireMatchInfo(t *testing.T, expected []*TagInfo, item *TagInfo) bool {
	for _, template := range expected {
		if template.Entity != item.Entity {
			continue
		}
		if template.Source != item.Source {
			continue
		}
		sort.Strings(template.LowCardTags)
		sort.Strings(item.LowCardTags)
		require.Equal(t, template.LowCardTags, item.LowCardTags)

		sort.Strings(template.HighCardTags)
		sort.Strings(item.HighCardTags)
		require.Equal(t, template.HighCardTags, item.HighCardTags)

		require.Equal(t, template.DeleteEntity, item.DeleteEntity)

		return true
	}

	t.Logf("could not find expected result for entity %s with sourcce %s", item.Entity, item.Source)
	return false
}

func TestKubeletPodTags(t *testing.T) {
	raw, err := ioutil.ReadFile("./test/kubelet/podlist_1.6.json")
	require.Nil(t, err)
	var podlist kubelet.PodList
	json.Unmarshal(raw, &podlist)

	raw, err = ioutil.ReadFile("./test/kubelet/podlist_1.6_result.json")
	require.Nil(t, err)
	var expected []*TagInfo
	json.Unmarshal(raw, &expected)
	require.Len(t, expected, 5)

	kubeCollector := &KubeletCollector{}

	infos, err := kubeCollector.parsePods(podlist.Items)

	// To export new result json
	//outJSON, err := json.Marshal(infos)
	//t.Logf("Results:\n%s", outJSON)

	require.Nil(t, err)
	require.Len(t, infos, 5)

	for _, item := range infos {
		t.Logf("testing entity %s", item.Entity)
		require.True(t, requireMatchInfo(t, expected, item))
	}
}

func TestKubeletPodLabelPrefix(t *testing.T) {
	raw, err := ioutil.ReadFile("./test/kubelet/podlist_1.6.json")
	require.Nil(t, err)
	var podlist kubelet.PodList
	json.Unmarshal(raw, &podlist)

	kubeCollector := &KubeletCollector{
		labelTagPrefix: "kube_",
	}

	infos, err := kubeCollector.parsePods(podlist.Items[0:1])
	result := infos[0]

	require.Contains(t, result.LowCardTags, "kube_app:dd-agent")
	require.NotContains(t, result.LowCardTags, "app:dd-agent")
}

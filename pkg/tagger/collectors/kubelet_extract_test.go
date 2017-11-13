// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubelet

package collectors

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/stretchr/testify/require"
)

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

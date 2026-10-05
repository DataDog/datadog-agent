// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && orchestrator && kubeapiserver && test

package pod

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.yaml.in/yaml/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func testNode(name string, nodeLabels map[string]string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: nodeLabels}}
}

func testPod(name, nodeName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID("uid-" + name), ResourceVersion: "1"},
		Spec:       corev1.PodSpec{NodeName: nodeName, Containers: []corev1.Container{{Name: "app", Image: "app"}}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func TestRunForSelectedNodes(t *testing.T) {
	cacheKey := cache.BuildAgentKey(constants.ClusterIDCacheKey)
	cachedClusterID, found := cache.Cache.Get(cacheKey)
	cache.Cache.Set(cacheKey, strings.Repeat("1", 36), cache.NoExpiration)
	t.Cleanup(func() {
		if found {
			cache.Cache.Set(cacheKey, cachedClusterID, cache.NoExpiration)
		} else {
			cache.Cache.Delete(cacheKey)
		}
	})

	fakeLabels := map[string]string{"kwok.x-k8s.io/node": "fake"}
	objects := []runtime.Object{
		testNode("kwok-a", fakeLabels),
		testNode("kwok-b", fakeLabels),
		testNode("real", nil),
		testPod("a1", "kwok-a"),
		testPod("a2", "kwok-a"),
		testPod("b1", "kwok-b"),
		testPod("r1", "real"),
		testPod("unassigned", ""),
	}
	client := fake.NewClientset(objects...)

	origGetKubeClient := getKubeClient
	getKubeClient = func() (kubernetes.Interface, error) { return client, nil }
	t.Cleanup(func() { getKubeClient = origGetKubeClient })

	mockConfig := configmock.New(t)
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	selector, err := labels.Parse("kwok.x-k8s.io/node=fake")
	require.NoError(t, err)

	sender := &fakeSender{}
	check := &Check{
		cfg:             mockConfig,
		sender:          sender,
		processor:       processors.NewProcessor(k8sProcessors.NewPodHandlers(mockConfig, mockStore, fakeTagger)),
		hostName:        "runner-host",
		config:          oconfig.NewDefaultOrchestratorConfig(nil),
		tagger:          fakeTagger,
		nodeSelector:    selector,
		clusterName:     "my-cluster",
		clusterNameTags: []string{"cluster_name:my-cluster"},
		systemInfo:      &process.SystemInfo{},
	}

	require.NoError(t, check.Run())

	require.Len(t, sender.pods, 2, "one payload per selected node")

	podNames := func(msg process.MessageBody) []string {
		var names []string
		for _, p := range msg.(*process.CollectorPod).Pods {
			names = append(names, p.Metadata.Name)
		}
		return names
	}

	first := sender.pods[0].(*process.CollectorPod)
	assert.Equal(t, "kwok-a-my-cluster", first.HostName)
	assert.ElementsMatch(t, []string{"a1", "a2"}, podNames(first))
	assert.ElementsMatch(t, []string{"kube_api_version:v1", "cluster_name:my-cluster"}, first.Tags)
	assert.Nil(t, first.Info, "system info of the host running the check must not be attributed to the node")

	second := sender.pods[1].(*process.CollectorPod)
	assert.Equal(t, "kwok-b-my-cluster", second.HostName)
	assert.ElementsMatch(t, []string{"b1"}, podNames(second))

	require.Len(t, sender.manifests, 2)
	assert.Equal(t, "kwok-a-my-cluster", sender.manifests[0].(*process.CollectorManifest).HostName)
}

func TestConfigureNodeSelector(t *testing.T) {
	for _, tc := range []struct {
		name     string
		instance string
		selector string
		wantErr  bool
	}{
		{name: "local node", instance: "{}"},
		{name: "selector", instance: "node_selector: kwok.x-k8s.io/node=fake", selector: "kwok.x-k8s.io/node=fake"},
		{name: "invalid selector", instance: "node_selector: 'a b'", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var instanceConfig checkConfig
			require.NoError(t, yaml.Unmarshal([]byte(tc.instance), &instanceConfig))
			selector, err := parseNodeSelector(instanceConfig.NodeSelector)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.selector == "" {
				assert.Nil(t, selector)
			} else {
				assert.Equal(t, tc.selector, selector.String())
			}
		})
	}
}

func TestValidateNodeSelectorPlacement(t *testing.T) {
	for _, tc := range []struct {
		name            string
		flavor          string
		provider        string
		hasNodeSelector bool
		wantErr         bool
	}{
		{name: "node agent", flavor: flavor.DefaultAgent, provider: names.File},
		{name: "node agent with node_selector", flavor: flavor.DefaultAgent, provider: names.File, hasNodeSelector: true, wantErr: true},
		{name: "cluster agent", flavor: flavor.ClusterAgent, provider: names.File, wantErr: true},
		{name: "cluster agent with node_selector", flavor: flavor.ClusterAgent, provider: names.File, hasNodeSelector: true},
		{name: "cluster check", flavor: flavor.DefaultAgent, provider: names.ClusterChecks, wantErr: true},
		{name: "cluster check with node_selector", flavor: flavor.DefaultAgent, provider: names.ClusterChecks, hasNodeSelector: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			origFlavor := flavor.GetFlavor()
			flavor.SetFlavor(tc.flavor)
			t.Cleanup(func() { flavor.SetFlavor(origFlavor) })

			err := validateNodeSelectorPlacement(tc.provider, tc.hasNodeSelector)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

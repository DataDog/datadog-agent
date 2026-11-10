// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	k8sCollectors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/helminformers"
)

// The Helm check and the orchestrator check watch the same Helm release
// objects. When they run in the same process they must share the informers.
func TestSharesInformersWithOrchestratorCollector(t *testing.T) {
	rls := &release{
		Name:      "my_datadog",
		Info:      &info{Status: "deployed"},
		Chart:     &chart{Metadata: &metadata{Name: "datadog", Version: "1.0.0"}},
		Version:   1,
		Namespace: "default",
	}
	cm, err := configMapForRelease(rls)
	require.NoError(t, err)
	client := fake.NewSimpleClientset(cm)

	check := newCheck().(*HelmCheck)
	check.kubeClient = client
	check.informersStopCh = make(chan struct{})
	require.NoError(t, check.setupInformers())

	stopCh := make(chan struct{})
	collector := k8sCollectors.NewHelmReleaseConfigMapCollector()
	collector.Init(&collectors.CollectorRunConfig{
		K8sCollectorRunConfig: collectors.K8sCollectorRunConfig{
			APIClient: &apiserver.APIClient{InformerCl: client},
		},
		StopCh: stopCh,
	})

	// Both consumers use the one informer the registry holds for the client.
	shared, release := helminformers.Acquire(client, helminformers.ConfigMaps)
	defer release()
	assert.Same(t, shared, collector.Informer())
	assert.Len(t, shared.GetStore().List(), 1)

	// The Helm check still gets the release through the shared informer.
	assert.Eventually(t, func() bool {
		return check.store.get(rls.namespacedName(), rls.revision(), k8sConfigmaps) != nil
	}, 5*time.Second, 10*time.Millisecond)

	// The informer stays up for the orchestrator collector when the Helm check stops...
	check.Cancel()
	assert.False(t, shared.IsStopped())

	// ...and goes away once the last holder, here the test, has released it.
	close(stopCh)
	release()
	assert.Eventually(t, shared.IsStopped, 5*time.Second, 10*time.Millisecond)
}

// A check canceled before its informers start must not leave them running.
func TestCancelBeforeSetupInformersReleasesInformers(t *testing.T) {
	client := fake.NewSimpleClientset()

	check := newCheck().(*HelmCheck)
	check.kubeClient = client
	check.informersStopCh = make(chan struct{})
	check.Cancel()
	require.NoError(t, check.setupInformers())

	// The check holds no reference: the informer stops with the last holder.
	informer, release := helminformers.Acquire(client, helminformers.ConfigMaps)
	release()
	assert.Eventually(t, informer.IsStopped, 5*time.Second, 10*time.Millisecond)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helm

import (
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/helminformers"
)

// The Helm check and the orchestrator check watch the same Helm release
// objects. When they run in the same process they must share the informers.
func TestSharesInformersWithOrchestratorCollector(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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

		// Another check, e.g. the orchestrator check, holds a synced informer before the Helm check joins it.
		shared, release := helminformers.Acquire(client, helminformers.ConfigMaps)
		defer release()
		require.True(t, cache.WaitForCacheSync(t.Context().Done(), shared.HasSynced))

		check := newCheck().(*HelmCheck)
		check.kubeClient = client
		check.informersStopCh = make(chan struct{})
		require.NoError(t, check.setupInformers())

		// The release is in the Helm check's store as soon as setup returns.
		assert.NotNil(t, check.store.get(rls.namespacedName(), rls.revision(), k8sConfigmaps))

		// The informer stays up for its other holder when the Helm check stops...
		check.Cancel()
		synctest.Wait()
		assert.False(t, shared.IsStopped())

		// ...and goes away once that holder releases it.
		release()
		synctest.Wait()
		assert.True(t, shared.IsStopped())
	})
}

// A check canceled before its informers start must not leave them running.
func TestCancelBeforeSetupInformersReleasesInformers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := fake.NewSimpleClientset()

		check := newCheck().(*HelmCheck)
		check.kubeClient = client
		check.informersStopCh = make(chan struct{})
		check.Cancel()
		require.NoError(t, check.setupInformers())

		// The check holds no reference: the informer stops with the last holder.
		informer, release := helminformers.Acquire(client, helminformers.ConfigMaps)
		release()
		synctest.Wait()
		assert.True(t, informer.IsStopped())
	})
}

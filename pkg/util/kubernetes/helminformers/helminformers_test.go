// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helminformers

import (
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func helmConfigMap(name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:          name,
			Namespace:     "default",
			Labels:        map[string]string{"owner": "helm"},
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "helm"}},
		},
		Data: map[string]string{"release": "payload"},
	}
}

func TestAcquireSharesInformer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := fake.NewClientset(helmConfigMap("sh.helm.release.v1.a.v1"))

		first, releaseFirst := Acquire(client, ConfigMaps)
		second, releaseSecond := Acquire(client, ConfigMaps)
		assert.Same(t, first, second)

		other, releaseOther := Acquire(client, Secrets)
		assert.NotSame(t, first, other)
		releaseOther()

		require.True(t, cache.WaitForCacheSync(t.Context().Done(), first.HasSynced))

		// The informer is trimmed, and still has the data consumers need.
		objs := first.GetStore().List()
		require.Len(t, objs, 1)
		cm := objs[0].(*corev1.ConfigMap)
		assert.Nil(t, cm.ManagedFields)
		assert.Equal(t, "payload", cm.Data["release"])

		// The informer outlives the first release, and stops with the last one.
		releaseFirst()
		releaseFirst() // releasing twice must not drop another holder's reference
		synctest.Wait()
		assert.False(t, first.IsStopped())
		releaseSecond()
		synctest.Wait()
		assert.True(t, first.IsStopped())

		// A new holder gets a fresh informer.
		again, releaseAgain := Acquire(client, ConfigMaps)
		defer releaseAgain()
		assert.NotSame(t, first, again)
	})
}

func TestAcquireDifferentClients(t *testing.T) {
	a, releaseA := Acquire(fake.NewClientset(), ConfigMaps)
	defer releaseA()
	b, releaseB := Acquire(fake.NewClientset(), ConfigMaps)
	defer releaseB()
	assert.NotSame(t, a, b)
}

func TestOwns(t *testing.T) {
	client := fake.NewClientset()
	informer, release := Acquire(client, ConfigMaps)
	assert.True(t, Owns(informer))

	other := cache.NewSharedIndexInformer(nil, &corev1.ConfigMap{}, 0, cache.Indexers{})
	assert.False(t, Owns(other))

	release()
	assert.False(t, Owns(informer))
}

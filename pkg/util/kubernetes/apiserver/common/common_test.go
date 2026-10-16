// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"context"
	stderrors "errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
)

func TestGetOrCreateClusterID(t *testing.T) {
	client := fake.NewSimpleClientset().CoreV1()

	// kube-system doesn't exist
	GetOrCreateClusterID(client)

	_, err := client.ConfigMaps(namespace.GetMyNamespace()).Get(context.TODO(), defaultClusterIDMap, metav1.GetOptions{})
	assert.True(t, errors.IsNotFound(err))

	// kube-system does exist
	kubeNs := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "123",
			UID:             "226430c6-5e57-11ea-91d5-42010a8400c6",
			Name:            "kube-system",
		},
	}
	client.Namespaces().Create(context.TODO(), &kubeNs, metav1.CreateOptions{})

	GetOrCreateClusterID(client)

	cm, err := client.ConfigMaps(namespace.GetMyNamespace()).Get(context.TODO(), defaultClusterIDMap, metav1.GetOptions{})
	assert.Nil(t, err)
	id, found := cm.Data["id"]
	assert.True(t, found)
	assert.Equal(t, "226430c6-5e57-11ea-91d5-42010a8400c6", id)
}

const testClusterID = "226430c6-5e57-11ea-91d5-42010a8400c6"

var configMapsResource = schema.GroupResource{Resource: "configmaps"}

// newClusterIDTestClient returns a fake client with the kube-system namespace and
// the given objects, and resets the cluster ID cache.
func newClusterIDTestClient(t *testing.T, objects ...runtime.Object) *fake.Clientset {
	cache.Cache.Flush()
	t.Cleanup(cache.Cache.Flush)
	previousBackoff := clusterIDBackoff
	clusterIDBackoff = wait.Backoff{Steps: previousBackoff.Steps, Duration: time.Millisecond}
	t.Cleanup(func() { clusterIDBackoff = previousBackoff })
	kubeSystem := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: testClusterID}}
	return fake.NewClientset(append(objects, kubeSystem)...)
}

func clusterIDConfigMap(data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: defaultClusterIDMap, Namespace: namespace.GetMyNamespace()},
		Data:       data,
	}
}

func countActions(client *fake.Clientset, verb string) int {
	count := 0
	for _, action := range client.Actions() {
		if action.GetVerb() == verb && action.GetResource().Resource == "configmaps" {
			count++
		}
	}
	return count
}

func TestGetOrCreateClusterIDConcurrentCreate(t *testing.T) {
	client := newClusterIDTestClient(t)
	// Another replica creates the ConfigMap between the Get and the Create.
	var once sync.Once
	client.PrependReactor("create", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
		handled := false
		once.Do(func() {
			require.NoError(t, client.Tracker().Add(clusterIDConfigMap(map[string]string{"id": testClusterID})))
			handled = true
		})
		if handled {
			return true, nil, errors.NewAlreadyExists(configMapsResource, defaultClusterIDMap)
		}
		return false, nil, nil
	})

	clusterID, err := GetOrCreateClusterID(client.CoreV1())

	require.NoError(t, err)
	assert.Equal(t, testClusterID, clusterID)
	cached, found := cache.Cache.Get(cache.BuildAgentKey(constants.ClusterIDCacheKey))
	assert.True(t, found)
	assert.Equal(t, testClusterID, cached)
}

func TestGetOrCreateClusterIDConcurrentUpdate(t *testing.T) {
	client := newClusterIDTestClient(t, clusterIDConfigMap(map[string]string{"id": "invalid"}))
	// Another replica fixes the ConfigMap between the Get and the Update.
	var once sync.Once
	client.PrependReactor("update", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
		handled := false
		once.Do(func() {
			require.NoError(t, client.Tracker().Update(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"},
				clusterIDConfigMap(map[string]string{"id": testClusterID}), namespace.GetMyNamespace()))
			handled = true
		})
		if handled {
			return true, nil, errors.NewConflict(configMapsResource, defaultClusterIDMap, stderrors.New("the object has been modified"))
		}
		return false, nil, nil
	})

	clusterID, err := GetOrCreateClusterID(client.CoreV1())

	require.NoError(t, err)
	assert.Equal(t, testClusterID, clusterID)
}

func TestGetOrCreateClusterIDConcurrentCalls(t *testing.T) {
	client := newClusterIDTestClient(t)

	const callers = 10
	clusterIDs := make([]string, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			clusterIDs[i], errs[i] = GetOrCreateClusterID(client.CoreV1())
		}()
	}
	wg.Wait()

	for i := range callers {
		require.NoError(t, errs[i])
		assert.Equal(t, testClusterID, clusterIDs[i])
	}
	assert.Equal(t, 1, countActions(client, "create"))
}

func TestGetOrCreateClusterIDNilData(t *testing.T) {
	client := newClusterIDTestClient(t, clusterIDConfigMap(nil))

	clusterID, err := GetOrCreateClusterID(client.CoreV1())

	require.NoError(t, err)
	assert.Equal(t, testClusterID, clusterID)
	cm, err := client.CoreV1().ConfigMaps(namespace.GetMyNamespace()).Get(context.TODO(), defaultClusterIDMap, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, testClusterID, cm.Data["id"])
}

func TestGetOrCreateClusterIDNotRetried(t *testing.T) {
	client := newClusterIDTestClient(t)
	client.PrependReactor("get", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.NewForbidden(configMapsResource, defaultClusterIDMap, stderrors.New("forbidden"))
	})

	_, err := GetOrCreateClusterID(client.CoreV1())

	assert.True(t, errors.IsForbidden(err))
	assert.Equal(t, 1, countActions(client, "get"))
}

func TestGetOrCreateClusterIDTransientError(t *testing.T) {
	client := newClusterIDTestClient(t)
	var once sync.Once
	client.PrependReactor("get", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
		handled := false
		once.Do(func() { handled = true })
		if handled {
			return true, nil, errors.NewServiceUnavailable("apiserver restarting")
		}
		return false, nil, nil
	})

	clusterID, err := GetOrCreateClusterID(client.CoreV1())

	require.NoError(t, err)
	assert.Equal(t, testClusterID, clusterID)
	assert.Equal(t, 2, countActions(client, "get"))
}

func TestGetOrCreateClusterIDFailureNotCached(t *testing.T) {
	client := newClusterIDTestClient(t)
	var forbidden atomic.Bool
	forbidden.Store(true)
	client.PrependReactor("get", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
		if forbidden.Load() {
			return true, nil, errors.NewForbidden(configMapsResource, defaultClusterIDMap, stderrors.New("forbidden"))
		}
		return false, nil, nil
	})

	_, err := GetOrCreateClusterID(client.CoreV1())
	require.Error(t, err)
	_, found := cache.Cache.Get(cache.BuildAgentKey(constants.ClusterIDCacheKey))
	assert.False(t, found)

	// The next call tries again.
	forbidden.Store(false)
	clusterID, err := GetOrCreateClusterID(client.CoreV1())
	require.NoError(t, err)
	assert.Equal(t, testClusterID, clusterID)
}

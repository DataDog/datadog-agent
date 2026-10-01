// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
)

func TestResolveClusterID(t *testing.T) {
	client := fake.NewSimpleClientset().CoreV1()

	// kube-system doesn't exist
	_, err := ResolveClusterID(t.Context(), client)
	require.Error(t, err)

	_, err = client.ConfigMaps(namespace.GetMyNamespace()).Get(context.TODO(), defaultClusterIDMap, metav1.GetOptions{})
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

	id, err := ResolveClusterID(t.Context(), client)
	require.NoError(t, err)
	assert.Equal(t, "226430c6-5e57-11ea-91d5-42010a8400c6", id)

	cm, err := client.ConfigMaps(namespace.GetMyNamespace()).Get(context.TODO(), defaultClusterIDMap, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "226430c6-5e57-11ea-91d5-42010a8400c6", cm.Data["id"])
}

func TestResolveClusterIDCancelsRequest(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer server.Close()
	client, err := typedcorev1.NewForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-started:
			cancel()
		case <-ctx.Done():
		}
	}()
	id, err := ResolveClusterID(ctx, client)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, id)
	<-canceled
}

func TestResolveClusterIDRepairsNilData(t *testing.T) {
	const expectedID = "226430c6-5e57-11ea-91d5-42010a8400c6"
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "datadog-cluster-id", Namespace: namespace.GetMyNamespace()}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: types.UID(expectedID)}},
	)
	id, err := ResolveClusterID(t.Context(), client.CoreV1())
	require.NoError(t, err)
	assert.Equal(t, expectedID, id)
	cm, err := client.CoreV1().ConfigMaps(namespace.GetMyNamespace()).Get(t.Context(), "datadog-cluster-id", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, expectedID, cm.Data["id"])
}

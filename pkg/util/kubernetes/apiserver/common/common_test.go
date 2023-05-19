// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetOrCreateClusterID(t *testing.T) {
	client := fake.NewSimpleClientset().CoreV1()

	// kube-system doesn't exist
	GetOrCreateClusterID(client)

	_, err := client.ConfigMaps("default").Get(context.TODO(), defaultClusterIDMap, metav1.GetOptions{})
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

	cm, err := client.ConfigMaps("default").Get(context.TODO(), defaultClusterIDMap, metav1.GetOptions{})
	assert.Nil(t, err)
	id, found := cm.Data["id"]
	assert.True(t, found)
	assert.Equal(t, "226430c6-5e57-11ea-91d5-42010a8400c6", id)
}

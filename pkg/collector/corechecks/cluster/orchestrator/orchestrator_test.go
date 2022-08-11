// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
)

// TestOrchestratorCheckStartupAndCleanup close simulates the check being closed and then restarted with .Start again
func TestOrchestratorCheckStartupAndCleanup(t *testing.T) {
	client := fake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(client, 0)
	cl := &apiserver.APIClient{Cl: client, InformerFactory: informerFactory, UnassignedPodInformerFactory: informerFactory}
	orchCheck := OrchestratorFactory().(*OrchestratorCheck)
	orchCheck.apiClient = cl

	bundle := NewCollectorBundle(orchCheck)
	err := bundle.Initialize()
	assert.NoError(t, err)

	// We will create an informer that writes added pods to a channel.
	nodes := make(chan *corev1.Node, 1)

	nodeInformer := informerFactory.Core().V1().Nodes().Informer()
	nodeInformer.AddEventHandler(&cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node := obj.(*corev1.Node)
			t.Logf("node added: %s/%s", node.Namespace, node.Name)
			nodes <- node
		},
	})

	writeNode(t, client, "1")
	select {
	case node := <-nodes:
		t.Logf("Got node from channel: %s/%s", node.Namespace, node.Name)
	case <-time.After(wait.ForeverTestTimeout):
		t.Error("Informer did not get the added node")
	}
}

func writeNode(t *testing.T, client *fake.Clientset, version string) {
	kubeN := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: version,
			UID:             types.UID("126430c6-5e57-11ea-91d5-42010a8400c6-" + version),
			Name:            "another-system-" + version,
		},
	}
	_, err := client.CoreV1().Nodes().Create(context.TODO(), &kubeN, metav1.CreateOptions{})
	assert.NoError(t, err)
}

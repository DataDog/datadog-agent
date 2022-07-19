/*
 * Unless explicitly stated otherwise all files in this repository are licensed
 * under the Apache License Version 2.0.
 * This product includes software developed at Datadog (https://www.datadoghq.com/).
 * Copyright 2016-2022 Datadog, Inc.
 */

package orchestrator

import (
	"context"
	"testing"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
)

// TestFactoryNotRestartedAfterClose close simulates the check being closed and then restarted with .Start again
func TestFactoryNotRestartedAfterClose(t *testing.T) {
	client := fake.NewSimpleClientset()
	stop := make(chan struct{})

	informerFactory := informers.NewSharedInformerFactory(client, 10)
	cl := &apiserver.APIClient{Cl: client, InformerFactory: informerFactory, UnassignedPodInformerFactory: informerFactory}
	check := OrchestratorCheck{
		CheckBase: core.CheckBase{},
		instance: &OrchestratorInstance{
			Collectors: []string{}, // empty means we will use the default list
		},
		stopCh:    stop,
		apiClient: cl,
	}

	bundle := NewCollectorBundle(&check)
	err := bundle.Initialize()
	assert.NoError(t, err)

	informer := informerFactory.Core().V1().Nodes().Informer()
	// empty
	keys := informer.GetStore().ListKeys()
	assert.Len(t, keys, 0)

	writeFirstNode(client)
	informer.GetIndexer().Resync()
	keys = informer.GetStore().ListKeys()
	assert.Len(t, keys, 1)

}

// TestOrchestratorCheckRaceWithDefaultResources makes sure that we are not running into a race condition during initialization
// by initializing the same informer multiple times.
func TestOrchestratorCheckRaceWithDefaultResources(t *testing.T) {
	client := fake.NewSimpleClientset()
	fac := informers.NewSharedInformerFactory(client, 10)
	cl := &apiserver.APIClient{Cl: client, InformerFactory: fac, UnassignedPodInformerFactory: fac}

	check := OrchestratorCheck{
		CheckBase: core.CheckBase{},
		instance: &OrchestratorInstance{
			Collectors: []string{}, // empty means we will use the default list
		},
		apiClient: cl,
	}

	bundle := NewCollectorBundle(&check)
	err := bundle.Initialize()
	assert.NoError(t, err)
}

func writeFirstNode(client *fake.Clientset) {
	kubeN := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "123",
			UID:             "226430c6-5e57-11ea-91d5-42010a8400c6",
			Name:            "kube-system",
		},
	}
	_, _ = client.CoreV1().Nodes().Create(context.TODO(), &kubeN, metav1.CreateOptions{})
}

func writeSecondNS(client *fake.Clientset) {
	kubeN := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "423",
			UID:             "126430c6-5e57-11ea-91d5-42010a8400c6",
			Name:            "another-system",
		},
	}
	_, _ = client.CoreV1().Nodes().Create(context.TODO(), &kubeN, metav1.CreateOptions{})
}

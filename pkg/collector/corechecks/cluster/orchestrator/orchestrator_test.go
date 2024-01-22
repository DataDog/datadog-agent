// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/inventory"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	crd "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	vpa "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func newCollectorBundle(chk *OrchestratorCheck) *CollectorBundle {
	bundle := &CollectorBundle{
		discoverCollectors: chk.orchestratorConfig.CollectorDiscoveryEnabled,
		check:              chk,
		inventory:          inventory.NewCollectorInventory(),
		runCfg: &collectors.CollectorRunConfig{
			APIClient:                   chk.apiClient,
			ClusterID:                   chk.clusterID,
			Config:                      chk.orchestratorConfig,
			MsgGroupRef:                 chk.groupID,
			OrchestratorInformerFactory: chk.orchestratorInformerFactory,
		},
		stopCh:              chk.stopCh,
		manifestBuffer:      NewManifestBuffer(chk),
		activatedCollectors: map[string]struct{}{},
	}
	bundle.importCollectorsFromInventory()
	bundle.prepareExtraSyncTimeout()
	return bundle
}

// TestOrchestratorCheckSafeReSchedule close simulates the check being unscheduled and rescheduled again
func TestOrchestratorCheckSafeReSchedule(t *testing.T) {
	var wg sync.WaitGroup

	client := fake.NewSimpleClientset()
	vpaClient := vpa.NewSimpleClientset()
	crdClient := crd.NewSimpleClientset()
	cl := &apiserver.APIClient{InformerCl: client, VPAInformerClient: vpaClient, CRDInformerClient: crdClient}
	orchCheck := newCheck().(*OrchestratorCheck)
	orchCheck.apiClient = cl

	orchCheck.orchestratorInformerFactory = getOrchestratorInformerFactory(cl)
	bundle := newCollectorBundle(orchCheck)
	err := bundle.Initialize()
	assert.NoError(t, err)

	wg.Add(2)

	// getting rescheduled.
	orchCheck.Cancel()

	bundle.runCfg.OrchestratorInformerFactory = getOrchestratorInformerFactory(cl)
	bundle.stopCh = make(chan struct{})
	err = bundle.Initialize()
	assert.NoError(t, err)

	_, err = bundle.runCfg.OrchestratorInformerFactory.InformerFactory.Core().V1().Nodes().Informer().AddEventHandler(&cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			wg.Done()
		},
	})
	assert.NoError(t, err)

	writeNode(t, client, "1")
	writeNode(t, client, "2")

	assert.True(t, waitTimeout(&wg, 2*time.Second))
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

// waitTimeout returns true if wg is completed and false if time is up
func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return true
	case <-time.After(timeout):
		return false
	}
}

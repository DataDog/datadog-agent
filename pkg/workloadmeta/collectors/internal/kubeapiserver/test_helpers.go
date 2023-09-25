// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"
)

const dummySubscriber = "dummy-subscriber"

func testCollectEvent(t *testing.T, createResource func(*fake.Clientset) error, newStore storeGenerator, expected []workloadmeta.EventBundle) {
	// Create a fake client to mock API calls.
	client := fake.NewSimpleClientset()

	// Use the fake client in kubeapiserver context.
	wlm := workloadmeta.NewMockStore()
	store, _ := newStore(context.TODO(), wlm, client)
	stopStore := make(chan struct{})
	go store.Run(stopStore)

	ch := wlm.Subscribe(dummySubscriber, workloadmeta.NormalPriority, nil)
	// When Subscribe is called, the first Bundle contains events about the items currently in the store.
	// In that case, the first bundle is empty.
	<-ch

	// Creating a resource
	err := createResource(client)
	assert.NoError(t, err)

	// Retrieving the resource in an event bundle
	bundle := <-ch
	close(bundle.Ch)
	// nil the bundle's Ch so we can
	// deep-equal just the events later
	bundle.Ch = nil
	actual := []workloadmeta.EventBundle{bundle}
	close(stopStore)
	wlm.Unsubscribe(ch)
	assert.Equal(t, expected, actual)
}

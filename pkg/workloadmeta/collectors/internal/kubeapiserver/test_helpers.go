// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"
)

const dummySubscriber = "dummy-subscriber"

func testCollectEvent(t *testing.T, createResource func(*fake.Clientset) error, newStore storeGenerator, expected workloadmeta.EventBundle) {
	// Create a fake client to mock API calls.
	client := fake.NewSimpleClientset()

	// Create a resource before starting the reflector store or workloadmeta so that if the reflector calls `List()` then
	// this resource can't be skipped
	err := createResource(client)
	assert.NoError(t, err)

	// Start the reflector
	wlm := workloadmeta.NewMockStore()
	store, _ := newStore(context.TODO(), wlm, client)
	stopStore := make(chan struct{})
	go store.Run(stopStore)

	// Subscribe to the kubeapiserver events. Two cases are possible:
	// - The reflector has already populated wlm with the resource, in that case the first call to <-ch will contain the event
	// - The reflector is still initializing. In that case the second call to <-ch will contain the event
	ch := wlm.Subscribe(dummySubscriber, workloadmeta.NormalPriority, nil)
	var bundle workloadmeta.EventBundle
	assert.Eventually(t, func() bool {
		select {
		case bundle = <-ch:
			close(bundle.Ch)
			if len(bundle.Events) == 0 {
				return false
			}
			// If bundle finally has an event, we can return from this
			return true

		default:
			return false
		}
	}, 30*time.Second, 500*time.Millisecond)

	// nil the bundle's Ch so we can
	// deep-equal just the events later
	bundle.Ch = nil
	actual := bundle
	assert.Equal(t, expected, actual)
	close(stopStore)
	wlm.Unsubscribe(ch)
}

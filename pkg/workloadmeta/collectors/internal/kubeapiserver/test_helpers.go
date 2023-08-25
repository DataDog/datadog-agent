// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"
)

const dummySubscriber = "dummy-subscriber"

func TestNewResourceStore(t *testing.T, newStore storeGenerator, cfg config.Config) error {
	mockClient := fake.NewSimpleClientset()
	ctx := context.Background()
	wlm := workloadmeta.NewMockStore()
	_, _, err := newStore(ctx, cfg, wlm, mockClient)
	return err
}

func TestFakeHelper(t *testing.T, cfg config.Config, createResource func(*fake.Clientset) error, newStore storeGenerator, expected []workloadmeta.EventBundle) {
	// Create a fake client to mock API calls.
	client := fake.NewSimpleClientset()

	// Creating a fake deployment
	err := createResource(client)
	assert.NoError(t, err)

	// Use the fake client in kubeapiserver context.
	wlm := workloadmeta.NewMockStore()
	ctx := context.Background()
	store, _, err := newStore(ctx, cfg, wlm, client)
	assert.NoError(t, err)
	stopStore := make(chan struct{})
	go store.Run(stopStore)

	ch := wlm.Subscribe(dummySubscriber, workloadmeta.NormalPriority, nil)
	doneCh := make(chan struct{})

	actual := []workloadmeta.EventBundle{}
	go func() {
		<-ch
		bundle := <-ch
		close(bundle.Ch)

		// nil the bundle's Ch so we can
		// deep-equal just the events later
		bundle.Ch = nil

		actual = append(actual, bundle)

		close(doneCh)
	}()

	<-doneCh
	close(stopStore)
	wlm.Unsubscribe(ch)
	assert.Equal(t, expected, actual)
}

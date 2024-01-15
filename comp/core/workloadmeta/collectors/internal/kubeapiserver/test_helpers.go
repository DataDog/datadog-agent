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

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
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

	overrides := map[string]interface{}{
		"cluster_agent.collect_kubernetes_tags": true,
		"language_detection.enabled":            true,
	}

	wlm := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Replace(config.MockParams{Overrides: overrides}),
		fx.Supply(context.Background()),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))
	ctx := context.TODO()
	wlm.Start(ctx)

	store, _ := newStore(ctx, wlm, client)
	stopStore := make(chan struct{})
	go store.Run(stopStore)

	// Subscribe to the kubeapiserver events. Two cases are possible:
	// - The reflector has already populated wlm with the resource, in that case the first call to <-ch will contain the event
	// - The reflector is still initializing. In that case the second call to <-ch will contain the event

	time.Sleep(5 * time.Second)

	ch := wlm.Subscribe(dummySubscriber, workloadmeta.NormalPriority, nil)
	var bundle workloadmeta.EventBundle
	read := assert.Eventually(t, func() bool {
		select {
		case bundle = <-ch:
			bundle.Acknowledge()
			if len(bundle.Events) == 0 {
				return false
			}
			// If bundle finally has an event, we can return from this
			return true

		default:
			return false
		}
	}, 30*time.Second, 500*time.Millisecond)

	// Retrieving the resource in an event bundle
	if !read {
		bundle = <-ch
		bundle.Acknowledge()
	}

	// nil the bundle's Ch so we can
	// deep-equal just the events later
	bundle.Ch = nil
	actual := bundle
	assert.Equal(t, expected, actual)
	close(stopStore)
	wlm.Unsubscribe(ch)
}

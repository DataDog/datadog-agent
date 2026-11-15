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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	metafake "k8s.io/client-go/metadata/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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
		"language_detection.reporting.enabled":  true,
	}

	wlm := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			return config.NewMockWithOverrides(t, overrides)
		}),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	reflector, store := newStore(wlm, wlm.GetConfig(), client)
	assertInitialBundle(t, wlm, reflector, store, expected)
}

func testCollectMetadataEvent(t *testing.T, createObjects func() []runtime.Object, gvr schema.GroupVersionResource, expected workloadmeta.EventBundle) {

	// Create a resource before starting the reflector store or workloadmeta so that if the reflector calls `List()` then
	// this resource can't be skipped

	// Create test scheme
	testScheme := runtime.NewScheme()
	// Register Metadata objects types to the test scheme
	err := v1.AddMetaToScheme(testScheme)
	assert.NoError(t, err)

	objects := createObjects()

	metadataclient := metafake.NewSimpleMetadataClient(testScheme, objects...)

	wlm := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	ctx := context.TODO()

	// Create a fake metadata client to mock API calls.
	_, err = metadataclient.Resource(gvr).List(ctx, v1.ListOptions{})
	assert.NoError(t, err)
	reflector, store := newMetadataStore(wlm, wlm.GetConfig(), metadataclient, gvr)
	assertInitialBundle(t, wlm, reflector, store, expected)
}

// assertInitialBundle runs the reflector until it has synced, then checks that
// the initial bundle of a new workloadmeta subscription matches expected.
func assertInitialBundle(t *testing.T, wlm workloadmetamock.Mock, reflector *cache.Reflector, store *reflectorStore, expected workloadmeta.EventBundle) {
	t.Helper()

	stopStore := make(chan struct{})
	defer close(stopStore)
	go reflector.Run(stopStore)

	// The reflector lists the resources in its own goroutine. Once the store
	// has synced, they are in workloadmeta, because the mock processes Notify
	// synchronously.
	require.Eventually(t, store.HasSynced, 5*time.Second, 10*time.Millisecond)

	// The initial bundle of a new subscription contains the entities already
	// in workloadmeta.
	ch := wlm.Subscribe(dummySubscriber, workloadmeta.NormalPriority, nil)
	defer wlm.Unsubscribe(ch)

	var bundle workloadmeta.EventBundle
	select {
	case bundle = <-ch:
		bundle.Acknowledge()
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timeout waiting for the initial event bundle")
	}

	// nil the bundle's Ch so we can
	// deep-equal just the events later
	bundle.Ch = nil
	bundle.EnqueuedAt = time.Time{}
	assert.Equal(t, expected, bundle)
}

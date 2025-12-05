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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func Test_PodsFakeKubernetesClient(t *testing.T) {
	t.Parallel()
	objectMeta := metav1.ObjectMeta{
		Name:   "test-pod",
		Labels: map[string]string{"test-label": "test-value"},
		UID:    types.UID("test-pod-uid"),
	}

	overrides := map[string]interface{}{
		"cluster_agent.collect_kubernetes_tags": true,
	}

	wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			return config.NewMockWithOverrides(t, overrides)
		}),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	store := newPodReflectorStore(wmeta, wmeta.GetConfig())

	ch := wmeta.Subscribe(dummySubscriber, workloadmeta.NormalPriority, nil)
	defer wmeta.Unsubscribe(ch)

	bundleCh := make(chan workloadmeta.EventBundle, 1)
	doneCh := make(chan struct{})
	defer close(doneCh)

	go func() {
		for {
			select {
			case bundle := <-ch:
				bundle.Acknowledge()
				if len(bundle.Events) > 0 {
					bundleCh <- bundle
					return
				}
			case <-doneCh:
				return
			}
		}
	}()

	pod := &MinimalPod{
		ObjectMeta: objectMeta,
		Spec:       MinimalPodSpec{Containers: []MinimalContainer{}},
	}
	err := store.Add(pod)
	require.NoError(t, err)

	var bundle workloadmeta.EventBundle
	select {
	case bundle = <-bundleCh:
		// Received bundle. Continue.
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	expected := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.KubernetesPod{
					Containers: []workloadmeta.OrchestratorContainer{},
					EntityID: workloadmeta.EntityID{
						ID:   string(objectMeta.UID),
						Kind: workloadmeta.KindKubernetesPod,
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      objectMeta.Name,
						Namespace: objectMeta.Namespace,
						Labels:    objectMeta.Labels,
					},
					Owners: []workloadmeta.KubernetesPodOwner{},
				},
			},
		},
	}

	bundle.Ch = nil // to avoid comparing the channel
	assert.Equal(t, expected, bundle)

}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubemetadata"
	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var timeout = 10 * time.Second
var interval = 50 * time.Millisecond

func Test_AddDelete_Deployment(t *testing.T) {
	workloadmetaComponent := mockedWorkloadmeta(t)

	deploymentStore := newDeploymentReflectorStore(workloadmetaComponent, workloadmetaComponent.GetConfig())

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"test-label": "test-value",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
		},
	}

	err := deploymentStore.Add(&deployment)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err = workloadmetaComponent.GetKubernetesDeployment("test-namespace/test-deployment")
		return err == nil
	}, timeout, interval)

	err = deploymentStore.Delete(&deployment)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err = workloadmetaComponent.GetKubernetesDeployment("test-namespace/test-deployment")
		return err != nil
	}, timeout, interval)
}

func Test_AddDelete_Pod(t *testing.T) {
	workloadmetaComponent := mockedWorkloadmeta(t)

	podStore := newPodReflectorStore(workloadmetaComponent, workloadmetaComponent.GetConfig())

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			UID:       "pod-uid",
			Labels: map[string]string{
				"test-label": "test-value",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
		},
	}

	err := podStore.Add(&pod)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err = workloadmetaComponent.GetKubernetesPod(string(pod.UID))
		return err == nil
	}, timeout, interval)

	err = podStore.Delete(&pod)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err = workloadmetaComponent.GetKubernetesDeployment(string(pod.UID))
		return err != nil
	}, timeout, interval)
}

func Test_AddDelete_PartialObjectMetadata(t *testing.T) {
	workloadmetaComponent := mockedWorkloadmeta(t)

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	}
	parser, err := kubernetesresourceparsers.NewMetadataParser(gvr, nil)
	require.NoError(t, err)

	metadataStore := &reflectorStore{
		wlmetaStore: workloadmetaComponent,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      parser,
	}

	partialObjMetadata := metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-object",
			Labels: map[string]string{
				"test-label": "test-value",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
		},
	}

	kubeMetadataEntityID := kubemetadata.GenerateEntityID("", "namespaces", "", "test-object")

	err = metadataStore.Add(&partialObjMetadata)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err = workloadmetaComponent.GetKubernetesMetadata(kubeMetadataEntityID)
		return err == nil
	}, timeout, interval)

	err = metadataStore.Delete(&partialObjMetadata)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err = workloadmetaComponent.GetKubernetesMetadata(kubeMetadataEntityID)
		return err != nil
	}, timeout, interval)
}

// This is a regression test. Unset events notified from Replace() had the
// expected workloadmeta kind but were always of type
// *workloadmeta.KubernetesPod instead of the expected type. This mismatch
// caused a panic in workloadmeta filters like the one used in this test
// (workloadmeta.IsNodeMetadata).
func TestReplace(t *testing.T) {
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "nodes",
	}

	testNodeMetadata := workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   string(kubemetadata.GenerateEntityID("", "nodes", "", "test-node")),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "test-node",
		},
		GVR: &gvr,
	}

	workloadmetaComponent := mockedWorkloadmeta(t)

	parser, err := kubernetesresourceparsers.NewMetadataParser(gvr, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithDeadline(context.TODO(), time.Now().Add(10*time.Second))
	defer cancel()

	receivedInitialBundle := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	// Create a goroutine that subscribes to workloadmeta and that has a filter
	// that will panic if the event sent to workloadmeta by Replace() is not of
	// the expected type.
	go func() {
		defer wg.Done()
		filter := workloadmeta.NewFilterBuilder().AddKindWithEntityFilter(
			workloadmeta.KindKubernetesMetadata,
			func(entity workloadmeta.Entity) bool {
				metadata := entity.(*workloadmeta.KubernetesMetadata)
				return workloadmeta.IsNodeMetadata(metadata)
			},
		).Build()

		wmetaEventsCh := workloadmetaComponent.Subscribe("test-subscriber", workloadmeta.NormalPriority, filter)
		defer workloadmetaComponent.Unsubscribe(wmetaEventsCh)

		var events []workloadmeta.Event

		for len(events) < 2 {
			select {
			case eventBundle := <-wmetaEventsCh:
				eventBundle.Acknowledge()

				if len(eventBundle.Events) == 0 {
					close(receivedInitialBundle)
					continue
				}

				events = append(events, eventBundle.Events...)
			case <-ctx.Done():
				require.FailNow(t, "timeout waiting for events")
			}
		}

		expectedEvents := []workloadmeta.Event{
			{
				Type:   workloadmeta.EventTypeSet,
				Entity: &testNodeMetadata,
			},
			{
				Type:   workloadmeta.EventTypeUnset,
				Entity: &testNodeMetadata,
			},
		}

		require.ElementsMatch(t, expectedEvents, events)
	}()

	metadataStore := &reflectorStore{
		wlmetaStore: workloadmetaComponent,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      parser,
	}

	partialObjMetadata := metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	// Wait until the goroutine has received the initial event that includes the
	// list of current entities (which should be empty at this point).
	// If we don't do this, there's the possibility of calling Add() and
	// Replace() before the goroutine processes the initial bundle. And because
	// Replace() deletes what Add() adds, the goroutine would not receive any
	// events, and we would not be able to check what we want in this test.
	<-receivedInitialBundle

	err = metadataStore.Add(&partialObjMetadata)
	require.NoError(t, err)

	err = metadataStore.Replace(nil, "")
	require.NoError(t, err)

	wg.Wait()
}

func mockedWorkloadmeta(t *testing.T) workloadmetamock.Mock {
	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
}

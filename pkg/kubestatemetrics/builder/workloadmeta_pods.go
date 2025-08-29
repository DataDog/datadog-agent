// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package builder

import (
	"context"
	"errors"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// workloadmetaReflector is a reflector that uses workloadmeta as the data source
// for pod information
type workloadmetaReflector struct {
	namespaces         []string
	watchAllNamespaces bool
	wmeta              workloadmeta.Component

	// Having an array of stores allows us to have a single reflector for all
	// the collectors configured (by default it's the pods one plus
	// "pods_extended")
	stores []cache.Store

	started bool
}

func newWorkloadmetaReflector(wmeta workloadmeta.Component, namespaces []string) (workloadmetaReflector, error) {
	if wmeta == nil {
		return workloadmetaReflector{}, errors.New("workloadmeta cannot be nil")
	}

	watchAllNamespaces := slices.Contains(namespaces, corev1.NamespaceAll)

	return workloadmetaReflector{
		namespaces:         namespaces,
		watchAllNamespaces: watchAllNamespaces,
		wmeta:              wmeta,
	}, nil
}

func (wr *workloadmetaReflector) addStore(store cache.Store) error {
	if wr.started {
		return errors.New("cannot add store after reflector has started")
	}

	wr.stores = append(wr.stores, store)
	return nil
}

// start starts the workloadmeta reflector. It should be called only once after all the
// stores have been added.
func (wr *workloadmetaReflector) start(ctx context.Context) error {
	if wr.started {
		return errors.New("reflector already started")
	}

	wmetaPodEvents := wr.wmeta.Subscribe("ksm", workloadmeta.NormalPriority, workloadmetaFilter())

	wr.started = true

	go wr.handleEvents(ctx, wmetaPodEvents)

	return nil
}

func workloadmetaFilter() *workloadmeta.Filter {
	return workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceNodeOrchestrator).
		AddKind(workloadmeta.KindKubernetesPod).
		Build()
}

func (wr *workloadmetaReflector) handleEvents(ctx context.Context, wmetaPodEvents chan workloadmeta.EventBundle) {
	for {
		select {
		case bundle := <-wmetaPodEvents:
			// No other component depends on this processing, so we can
			// acknowledge before processing to avoid blocking workloadmeta.
			bundle.Acknowledge()
			wr.processEventBundle(bundle)

		case <-ctx.Done():
			log.Debug("workloadmeta reflector context cancelled, stopping event handler")
			wr.wmeta.Unsubscribe(wmetaPodEvents)
			return
		}
	}
}

func (wr *workloadmetaReflector) processEventBundle(bundle workloadmeta.EventBundle) {
	for _, event := range bundle.Events {
		switch event.Type {
		case workloadmeta.EventTypeSet:
			wr.handlePodSetEvent(event)
		case workloadmeta.EventTypeUnset:
			wr.handlePodUnsetEvent(event)
		}
	}
}

func (wr *workloadmetaReflector) handlePodSetEvent(event workloadmeta.Event) {
	kubernetesPod, ok := event.Entity.(*workloadmeta.KubernetesPod)
	if !ok {
		log.Warnf("Expected KubernetesPod, got %T", event.Entity)
		return
	}

	if !wr.watchAllNamespaces && !slices.Contains(wr.namespaces, kubernetesPod.EntityMeta.Namespace) {
		return
	}

	k8sPod := convertWorkloadmetaPodToK8sPod(kubernetesPod, wr.wmeta)

	for _, store := range wr.stores {
		if err := store.Add(k8sPod); err != nil {
			// log instead of returning error to continue updating other stores
			log.Warnf("Failed to add pod %s to store: %s", kubernetesPod.EntityID.ID, err)
		}
	}
}

func (wr *workloadmetaReflector) handlePodUnsetEvent(event workloadmeta.Event) {
	podID := event.Entity.GetID()

	// Create a minimal pod object for deletion (only UID is needed)
	k8sPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID: types.UID(podID.ID),
		},
	}

	for _, store := range wr.stores {
		if err := store.Delete(k8sPod); err != nil {
			// log instead of returning error to continue updating other stores
			log.Warnf("Failed to delete pod %s from store: %s", podID.ID, err)
		}
	}
}

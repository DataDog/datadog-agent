// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package builder

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	podWatcherExpiryDuration = 15 * time.Second
	updateStoresPeriod       = 5 * time.Second
)

// podWatcher is an interface for a component that watches for changes in pods
type podWatcher interface {
	PullChanges(ctx context.Context) ([]*kubelet.Pod, error)
	Expire() ([]string, error)
}

type kubeletReflector struct {
	namespaces         []string
	watchAllNamespaces bool
	podWatcher         podWatcher

	// Having an array of stores allows us to have a single watcher for all the
	// collectors configured (by default it's the pods one plus "pods_extended")
	stores []cache.Store

	started bool
}

func newKubeletReflector(namespaces []string) (kubeletReflector, error) {
	watcher, err := kubelet.NewPodWatcher(podWatcherExpiryDuration)
	if err != nil {
		return kubeletReflector{}, fmt.Errorf("failed to create kubelet-based reflector: %w", err)
	}

	watchAllNamespaces := slices.Contains(namespaces, corev1.NamespaceAll)

	return kubeletReflector{
		namespaces:         namespaces,
		watchAllNamespaces: watchAllNamespaces,
		podWatcher:         watcher,
	}, nil
}

func (kr *kubeletReflector) addStore(store cache.Store) error {
	if kr.started {
		return fmt.Errorf("cannot add store after reflector has started")
	}

	kr.stores = append(kr.stores, store)

	return nil
}

// start starts the reflector. It should be called only once after all the
// stores have been added
func (kr *kubeletReflector) start(context context.Context) error {
	if kr.started {
		return fmt.Errorf("reflector already started")
	}

	kr.started = true

	ticker := time.NewTicker(updateStoresPeriod)

	go func() {
		for {
			select {
			case <-ticker.C:
				err := kr.updateStores(context)
				if err != nil {
					log.Errorf("Failed to update stores: %s", err)
				}

			case <-context.Done():
				ticker.Stop()
				return
			}
		}
	}()

	return nil
}

func (kr *kubeletReflector) updateStores(ctx context.Context) error {
	pods, err := kr.podWatcher.PullChanges(ctx)
	if err != nil {
		return fmt.Errorf("failed to pull changes from pod watcher: %w", err)
	}

	for _, pod := range pods {
		if !kr.watchAllNamespaces && !slices.Contains(kr.namespaces, pod.Metadata.Namespace) {
			continue
		}

		kubePod := kubelet.ConvertKubeletPodToK8sPod(pod)

		for _, store := range kr.stores {
			err := store.Add(kubePod)
			if err != nil {
				// log instead of returning error to continue updating other stores
				log.Warnf("Failed to add pod to store: %s", err)
			}
		}
	}

	expiredEntities, err := kr.podWatcher.Expire()
	if err != nil {
		return fmt.Errorf("failed to expire pods: %w", err)
	}

	for _, expiredEntity := range expiredEntities {
		// Expire() returns both pods and containers, we only care
		// about pods
		if !strings.HasPrefix(expiredEntity, kubelet.KubePodPrefix) {
			continue
		}

		// Only the UID is needed to be able to delete
		expiredPod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				UID: types.UID(strings.TrimPrefix(expiredEntity, kubelet.KubePodPrefix)),
			},
		}

		for _, store := range kr.stores {
			err := store.Delete(&expiredPod)
			if err != nil {
				// log instead of returning error to continue updating other stores
				log.Warnf("Failed to delete pod from store: %s", err)
			}
		}
	}

	return nil
}

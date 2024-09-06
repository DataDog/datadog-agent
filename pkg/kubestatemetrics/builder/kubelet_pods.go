// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package builder

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PodWatcher is an interface for a component that watches for changes in pods
type PodWatcher interface {
	PullChanges(ctx context.Context) ([]*kubelet.Pod, error)
	Expire() ([]string, error)
}

func (b *Builder) startKubeletPodWatcher(store cache.Store, namespace string) {
	podWatcher, err := kubelet.NewPodWatcher(15 * time.Second)
	if err != nil {
		log.Warnf("Failed to create pod watcher: %s", err)
	}

	ticker := time.NewTicker(5 * time.Second)

	go func() {
		for {
			select {
			case <-ticker.C:
				err = updateStore(b.ctx, store, podWatcher, namespace)
				if err != nil {
					log.Errorf("Failed to update store: %s", err)
				}

			case <-b.ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

func updateStore(ctx context.Context, store cache.Store, podWatcher PodWatcher, namespace string) error {
	pods, err := podWatcher.PullChanges(ctx)
	if err != nil {
		return fmt.Errorf("failed to pull changes from pod watcher: %w", err)
	}

	for _, pod := range pods {
		if namespace != corev1.NamespaceAll && pod.Metadata.Namespace != namespace {
			continue
		}

		kubePod := kubelet.ConvertKubeletPodToK8sPod(pod)

		err = store.Add(kubePod)
		if err != nil {
			log.Warnf("Failed to add pod to KSM store: %s", err)
		}
	}

	expiredEntities, err := podWatcher.Expire()
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

		err = store.Delete(&expiredPod)
		if err != nil {
			log.Warnf("Failed to delete pod from KSM store: %s", err)
		}
	}

	return nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package nginx

import (
	"context"
	"fmt"
	"maps"
	"strings"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// configMapReconciler watches original ConfigMaps labeled with
// appsec.datadoghq.com/watched-configmap=true and re-merges DD directives
// into the corresponding DD-owned copy whenever the original is updated.
// It keeps no in-memory state; all context is derived from ConfigMap
// labels and annotations at reconcile time.
type configMapReconciler struct {
	client dynamic.Interface
	logger log.Component
	config appsecconfig.Config
}

type reconcileItem struct {
	namespace    string
	originalName string
}

// Start launches the ConfigMap reconciler goroutine and returns immediately.
// The goroutine is cancelled when ctx is done.
func (r *configMapReconciler) Start(ctx context.Context) error {
	queue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.NewTypedItemExponentialFailureRateLimiter[reconcileItem](
			r.config.BaseBackoff,
			r.config.MaxBackoff,
		),
		workqueue.TypedRateLimitingQueueConfig[reconcileItem]{
			Name: "appsec_nginx_configmap_reconciler",
		},
	)

	informerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(r.client, 0, metav1.NamespaceAll, func(options *metav1.ListOptions) {
		options.LabelSelector = watchedConfigMapLabel + "=true"
	})

	informer := informerFactory.ForResource(configMapGVR).Informer()

	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(_, newObj any) {
			u, ok := newObj.(*unstructured.Unstructured)
			if !ok {
				return
			}
			queue.Add(reconcileItem{
				namespace:    u.GetNamespace(),
				originalName: u.GetName(),
			})
		},
	}); err != nil {
		queue.ShutDown()
		return fmt.Errorf("failed to add ConfigMap reconciler event handler: %w", err)
	}

	go func() {
		defer queue.ShutDown()

		informer.Run(ctx.Done())
	}()

	go func() {
		// Wait for the informer cache to sync before processing items.
		if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
			r.logger.Warn("ConfigMap reconciler: timed out waiting for informer cache sync")
			return
		}

		r.logger.Info("ConfigMap reconciler started")
		for {
			item, quit := queue.Get()
			if quit {
				return
			}
			r.reconcile(ctx, queue, item)
		}
	}()

	return nil
}

func (r *configMapReconciler) reconcile(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcileItem], item reconcileItem) {
	defer queue.Done(item)

	// Re-read the original ConfigMap to get the DD CM name from its annotation.
	original, err := r.client.Resource(configMapGVR).Namespace(item.namespace).Get(ctx, item.originalName, metav1.GetOptions{})
	if err != nil {
		r.logger.Warnf("ConfigMap reconciler: failed to get original ConfigMap %s/%s: %v", item.namespace, item.originalName, err)
		queue.Forget(item)
		return
	}

	annotations := original.GetAnnotations()
	ddCMName := annotations[ddConfigMapAnnotation]
	if ddCMName == "" {
		r.logger.Debugf("ConfigMap reconciler: original ConfigMap %s/%s has no DD ConfigMap annotation, skipping", item.namespace, item.originalName)
		queue.Forget(item)
		return
	}

	// Derive the original CM name from the DD CM name for validation.
	expectedOriginalName := strings.TrimPrefix(ddCMName, ddConfigMapPrefix)
	if expectedOriginalName != item.originalName {
		r.logger.Warnf("ConfigMap reconciler: annotation mismatch on %s/%s: expected original name %q from DD CM %q", item.namespace, item.originalName, expectedOriginalName, ddCMName)
		queue.Forget(item)
		return
	}

	moduleMountPath := r.config.Nginx.ModuleMountPath

	// Build labels the same way MutatePod does.
	ddLabels := make(map[string]string, len(r.config.CommonLabels)+1)
	maps.Copy(ddLabels, r.config.CommonLabels)
	ddLabels[appsecconfig.AppsecProcessorProxyTypeAnnotation] = string(appsecconfig.ProxyTypeIngressNginx)

	if err := createOrUpdateDDConfigMap(ctx, r.client, item.namespace, item.originalName, moduleMountPath, ddLabels, r.config.CommonAnnotations); err != nil {
		r.logger.Warnf("ConfigMap reconciler: failed to re-merge DD ConfigMap for %s/%s: %v", item.namespace, item.originalName, err)
		if queue.NumRequeues(item) < 5 {
			queue.AddRateLimited(item)
		} else {
			r.logger.Errorf("ConfigMap reconciler: giving up on %s/%s after retries", item.namespace, item.originalName)
			queue.Forget(item)
		}
		return
	}

	r.logger.Infof("ConfigMap reconciler: re-merged DD ConfigMap %s/%s from original %s", item.namespace, ddCMName, item.originalName)
	queue.Forget(item)
}

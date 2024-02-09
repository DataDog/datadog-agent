// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/watermarkpodautoscaler/api/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SyncInformers should be called after the instantiation of new informers.
// It's blocking until the informers are synced or the timeout exceeded.
// An extra timeout duration can be provided depending on the informer
func SyncInformers(informers map[InformerName]cache.SharedInformer, extraWait time.Duration) error {
	var g errgroup.Group
	timeoutConfig := config.Datadog.GetDuration("kube_cache_sync_timeout_seconds") * time.Second
	// syncTimeout can be used to wait for the kubernetes client-go cache to sync.
	// It cannot be retrieved at the package-level due to the package being imported before configs are loaded.
	syncTimeout := timeoutConfig + extraWait
	for name := range informers {
		name := name // https://golang.org/doc/faq#closures_and_goroutines
		g.Go(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
			defer cancel()
			start := time.Now()
			if !cache.WaitForCacheSync(ctx.Done(), informers[name].HasSynced) {
				end := time.Now()
				cacheSyncTimeouts.Inc()
				log.Warnf("couldn't sync informer %s in %v (kube_cache_sync_timeout_seconds: %v)", name, end.Sub(start), timeoutConfig)
				return fmt.Errorf("couldn't sync informer %s in %v", name, end.Sub(start))
			}
			log.Debugf("Sync done for informer %s in %v, last resource version: %s", name, time.Since(start), informers[name].LastSyncResourceVersion())
			return nil
		})
	}
	return g.Wait()
}

type syncInformerResult struct {
	informerName InformerName
	err          error
}

// SyncInformersReturnErrors does the same thing as SyncInformers except it returns a map of InformerName and error
func SyncInformersReturnErrors(informers map[InformerName]cache.SharedInformer, extraWait time.Duration) map[InformerName]error {
	resultChan := make(chan syncInformerResult)
	errors := make(map[InformerName]error, len(informers))
	timeoutConfig := config.Datadog.GetDuration("kube_cache_sync_timeout_seconds") * time.Second
	// syncTimeout can be used to wait for the kubernetes client-go cache to sync.
	// It cannot be retrieved at the package-level due to the package being imported before configs are loaded.
	syncTimeout := timeoutConfig + extraWait
	for name := range informers {
		name := name // https://golang.org/doc/faq#closures_and_goroutines
		go (func() {
			ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
			defer cancel()
			start := time.Now()
			if !cache.WaitForCacheSync(ctx.Done(), informers[name].HasSynced) {
				end := time.Now()
				cacheSyncTimeouts.Inc()
				log.Warnf("couldn't sync informer %s in %v (kube_cache_sync_timeout_seconds: %v)", name, end.Sub(start), timeoutConfig)
				resultChan <- syncInformerResult{
					informerName: name,
					err:          fmt.Errorf("couldn't sync informer %s in %v", name, end.Sub(start)),
				}
				return
			}
			log.Debugf("Sync done for informer %s in %v, last resource version: %s", name, time.Since(start), informers[name].LastSyncResourceVersion())
			resultChan <- syncInformerResult{
				informerName: name,
				err:          nil,
			}
		})()
	}

	for r := range resultChan {
		errors[r.informerName] = r.err
		if len(errors) == len(informers) {
			break
		}
	}

	return errors
}

// UnstructuredIntoWPA converts an unstructured into a WPA
func UnstructuredIntoWPA(obj interface{}, structDest *v1alpha1.WatermarkPodAutoscaler) error {
	unstrObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("could not cast Unstructured object: %v", obj)
	}
	return runtime.DefaultUnstructuredConverter.FromUnstructured(unstrObj.UnstructuredContent(), structDest)
}

// UnstructuredFromAutoscaler converts a WPA object into an Unstructured
func UnstructuredFromAutoscaler(structIn runtime.Object, unstructOut *unstructured.Unstructured) error {
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(structIn)
	if err != nil {
		return fmt.Errorf("Unable to convert WatermarkPodAutoscaler %v: %w", structIn, err)
	}
	unstructOut.SetUnstructuredContent(content)
	return nil
}

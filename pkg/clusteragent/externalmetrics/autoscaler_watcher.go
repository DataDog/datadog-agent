// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package externalmetrics

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	wpa_informer "github.com/DataDog/watermarkpodautoscaler/pkg/client/informers/externalversions"
	wpa_lister "github.com/DataDog/watermarkpodautoscaler/pkg/client/listers/datadoghq/v1alpha1"

	autoscaler "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	autoscaler_lister "k8s.io/client-go/listers/autoscaling/v2beta1"
	"k8s.io/client-go/tools/cache"
)

type AutoscalerWatcher struct {
	refreshPeriod          int64
	autoscalerLister       autoscaler_lister.HorizontalPodAutoscalerLister
	autoscalerListerSynced cache.InformerSynced
	wpaLister              wpa_lister.WatermarkPodAutoscalerLister
	wpaListerSynced        cache.InformerSynced
	le                     apiserver.LeaderElectorInterface
	store                  *DatadogMetricsInternalStore
}

// NewAutoscalerWatcher returns a new AutoscalerWatcher, giving nil `autoscalerInformer` or nil `wpaInformer` disables watching HPA or WPA
// We need at least one of them
func NewAutoscalerWatcher(refreshPeriod int64, informer informers.SharedInformerFactory, wpaInformer wpa_informer.SharedInformerFactory, le apiserver.LeaderElectorInterface, store *DatadogMetricsInternalStore) (*AutoscalerWatcher, error) {
	if store == nil {
		return nil, fmt.Errorf("Store cannot be nil")
	}

	// Check that we have at least one valid resource to watch
	if informer == nil && wpaInformer == nil {
		return nil, fmt.Errorf("Must enable at least HPA or WPA")
	}

	// Setup HPA
	var autoscalerLister autoscaler_lister.HorizontalPodAutoscalerLister
	var autoscalerListerSynced cache.InformerSynced
	if informer != nil {
		autoscalerLister = informer.Autoscaling().V2beta1().HorizontalPodAutoscalers().Lister()
		autoscalerListerSynced = informer.Autoscaling().V2beta1().HorizontalPodAutoscalers().Informer().HasSynced
	}

	// Setup WPA
	var wpaLister wpa_lister.WatermarkPodAutoscalerLister
	var wpaListerSynced cache.InformerSynced
	if wpaInformer != nil {
		wpaLister = wpaInformer.Datadoghq().V1alpha1().WatermarkPodAutoscalers().Lister()
		wpaListerSynced = wpaInformer.Datadoghq().V1alpha1().WatermarkPodAutoscalers().Informer().HasSynced
	}

	autoscalerWatcher := &AutoscalerWatcher{
		refreshPeriod:          refreshPeriod,
		autoscalerLister:       autoscalerLister,
		autoscalerListerSynced: autoscalerListerSynced,
		wpaLister:              wpaLister,
		wpaListerSynced:        wpaListerSynced,
		le:                     le,
		store:                  store,
	}

	return autoscalerWatcher, nil
}

func (w *AutoscalerWatcher) Run(stopCh <-chan struct{}) {
	log.Infof("Starting AutoscalerWatcher")
	if w.autoscalerListerSynced != nil {
		cache.WaitForCacheSync(stopCh, w.autoscalerListerSynced)
	}

	if w.wpaListerSynced != nil {
		cache.WaitForCacheSync(stopCh, w.wpaListerSynced)
	}
	log.Infof("AutoscalerWatcher started")

	tickerRefreshProcess := time.NewTicker(time.Duration(w.refreshPeriod) * time.Second)
	for {
		select {
		case <-tickerRefreshProcess.C:
			if w.le.IsLeader() {
				w.updateAutoscalerReferences()
			}
		case <-stopCh:
			log.Infof("Stopping AutoscalerWatcher")
			return
		}
	}
}

func (w *AutoscalerWatcher) updateAutoscalerReferences() {
	log.Debugf("Refreshing AutoscalerReferences")
	datadogMetricReferences := make(map[string]struct{}, w.store.Count())

	if w.autoscalerLister != nil {
		hpaList, err := w.autoscalerLister.HorizontalPodAutoscalers(metav1.NamespaceAll).List(labels.Everything())
		if err != nil {
			log.Errorf("Could not list HPAs (to update DatadogMetric active status): %v", err)
			return
		}

		for _, hpa := range hpaList {
			for _, metric := range hpa.Spec.Metrics {
				if metric.Type == autoscaler.ExternalMetricSourceType && metric.External != nil {
					if datadogMetricId, ok := metricNameToDatadogMetricId(metric.External.MetricName); ok {
						datadogMetricReferences[datadogMetricId] = struct{}{}
					}
				}
			}
		}
	}

	if w.wpaLister != nil {
		wpaList, err := w.wpaLister.WatermarkPodAutoscalers(metav1.NamespaceAll).List(labels.Everything())
		if err != nil {
			log.Errorf("Could not list WPAs (to update DatadogMetric active status): %v", err)
			return
		}

		for _, wpa := range wpaList {
			for _, metric := range wpa.Spec.Metrics {
				if metric.External != nil {
					if datadogMetricId, ok := metricNameToDatadogMetricId(metric.External.MetricName); ok {
						datadogMetricReferences[datadogMetricId] = struct{}{}
					}
				}
			}
		}
	}

	// Go through all DatadogMetric and check if we have a reference or not
	for _, datadogMetric := range w.store.GetAll() {
		_, active := datadogMetricReferences[datadogMetric.Id]
		if active != datadogMetric.Active {
			log.Debugf("Updating active status for: %s to: %t", datadogMetric.Id, active)

			if currentDatadogMetric := w.store.LockRead(datadogMetric.Id, false); currentDatadogMetric != nil {
				currentDatadogMetric.UpdateTime = time.Now().UTC()
				currentDatadogMetric.Active = active
				// If we move from Active to Inactive, we discard current valid state to avoid using unrefreshed metrics upon re-activation
				if !currentDatadogMetric.Active {
					currentDatadogMetric.Valid = false
				}

				w.store.UnlockSet(currentDatadogMetric.Id, *currentDatadogMetric)
			}
		}
	}
}

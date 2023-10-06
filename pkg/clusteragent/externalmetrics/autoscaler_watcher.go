// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"fmt"
	"strings"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	autoscalingv2beta1 "k8s.io/api/autoscaling/v2beta1"
	autoscalingv2beta2 "k8s.io/api/autoscaling/v2beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamic_informer "k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/watermarkpodautoscaler/api/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	autoscalerWatcherStoreID    string = "aw"
	autoscalerReferencesSep     string = ", "
	autoscalerReferencesKindSep string = ":"
	autoscalerWPAKindKey        string = "wpa"
	autoscalerHPAKindKey        string = "hpa"
)

// AutoscalerWatcher watches autoscaling objects and reconciles the corresponding external metrics
type AutoscalerWatcher struct {
	refreshPeriod           int64
	autogenEnabled          bool
	autogenExpirationPeriod time.Duration
	autogenNamespace        string
	autoscalerLister        cache.GenericLister
	autoscalerListerSynced  cache.InformerSynced
	wpaLister               cache.GenericLister
	wpaListerSynced         cache.InformerSynced
	isLeader                func() bool
	store                   *DatadogMetricsInternalStore
}

type externalMetric struct {
	metricName           string
	metricLabels         map[string]string
	autoscalerReferences []string
}

var gvr = schema.GroupVersionResource{
	Group:    "datadoghq.com",
	Version:  "v1alpha1",
	Resource: "watermarkpodautoscalers",
}

// NewAutoscalerWatcher returns a new AutoscalerWatcher, giving nil `autoscalerInformer` or nil `wpaInformer` disables watching HPA or WPA
// We need at least one of them
func NewAutoscalerWatcher(
	refreshPeriod int64,
	autogenEnabled bool,
	autogenExpirationPeriodHours int64,
	autogenNamespace string,
	client kubernetes.Interface,
	informer informers.SharedInformerFactory,
	wpaInformer dynamic_informer.DynamicSharedInformerFactory,
	isLeader func() bool,
	store *DatadogMetricsInternalStore,
) (*AutoscalerWatcher, error) {
	if store == nil {
		return nil, fmt.Errorf("Store must be initialized")
	}

	// Check that we have at least one valid resource to watch
	if informer == nil && wpaInformer == nil {
		return nil, fmt.Errorf("Must enable at least HPA or WPA")
	}

	// Setup HPA
	var autoscalerLister cache.GenericLister
	var autoscalerListerSynced cache.InformerSynced
	if informer != nil {
		hpaGVR, err := autoscalers.DiscoverHPAGroupVersionResource(client)
		if err != nil {
			return nil, fmt.Errorf("unable to discover HPA GroupVersionResource: %s", err)
		}

		genericInformerFactory, err := informer.ForResource(hpaGVR)
		if err != nil {
			return nil, fmt.Errorf("error creating generic informer: %s", err)
		}

		autoscalerLister = genericInformerFactory.Lister()
		autoscalerListerSynced = genericInformerFactory.Informer().HasSynced

	}

	// Setup WPA
	var wpaLister cache.GenericLister
	var wpaListerSynced cache.InformerSynced
	if wpaInformer != nil {
		wpaLister = wpaInformer.ForResource(gvr).Lister()
		wpaListerSynced = wpaInformer.ForResource(gvr).Informer().HasSynced
	}

	autoscalerWatcher := &AutoscalerWatcher{
		refreshPeriod:           refreshPeriod,
		autogenEnabled:          autogenEnabled,
		autogenExpirationPeriod: time.Duration(autogenExpirationPeriodHours) * time.Hour,
		autogenNamespace:        autogenNamespace,
		autoscalerLister:        autoscalerLister,
		autoscalerListerSynced:  autoscalerListerSynced,
		wpaLister:               wpaLister,
		wpaListerSynced:         wpaListerSynced,
		isLeader:                isLeader,
		store:                   store,
	}

	return autoscalerWatcher, nil
}

// Run starts the autoscaling reconciliation loop
func (w *AutoscalerWatcher) Run(stopCh <-chan struct{}) {
	log.Infof("Starting AutoscalerWatcher (waiting for cache sync)")
	if w.autoscalerListerSynced != nil {
		cache.WaitForCacheSync(stopCh, w.autoscalerListerSynced)
	}

	if w.wpaListerSynced != nil {
		cache.WaitForCacheSync(stopCh, w.wpaListerSynced)
	}
	log.Infof("AutoscalerWatcher started (cache sync finished)")

	tickerRefreshProcess := time.NewTicker(time.Duration(w.refreshPeriod) * time.Second)
	for {
		select {
		case <-tickerRefreshProcess.C:
			if w.isLeader() {
				w.processAutoscalers()
			}
		case <-stopCh:
			log.Infof("Stopping AutoscalerWatcher")
			return
		}
	}
}

func (w *AutoscalerWatcher) processAutoscalers() {
	log.Debugf("Refreshing Autoscaler references/Autogenerated metrics")
	datadogMetricReferences, err := w.getAutoscalerReferences()
	if err != nil {
		log.Errorf("Unable to refresh Autoscalers state: %v", err)
		return
	}

	// Go through all DatadogMetric and perform necessary actions
	for _, datadogMetric := range w.store.GetAll() {
		var autoscalerReferences string

		externalMetric, active := datadogMetricReferences[datadogMetric.ID]
		if externalMetric != nil {
			autoscalerReferences = strings.Join(externalMetric.autoscalerReferences, autoscalerReferencesSep)
		}

		// Make sure we don't de-activate metrics that are forced to be always active
		if datadogMetric.AlwaysActive {
			active = true
		}

		// Update DatadogMetric active status
		w.updateDatadogMetricStatus(active, autoscalerReferences, datadogMetric)

		// Delete autogen DatadogMetrics that haven't been updated for some time
		w.cleanupAutogenDatadogMetric(active, datadogMetric)

		// We clean reference map to keep references only existing on Kubernetes side
		delete(datadogMetricReferences, datadogMetric.ID)
	}

	// In `datadogMetricReferences` we now only have existing references that we should create
	// Or autoscalers referencing inexisting DatadogMetrics (in this case, externalMetric is nil)
	for datadogMetricID, externalMetric := range datadogMetricReferences {
		if externalMetric != nil && len(externalMetric.metricName) > 0 {
			autogenQuery := buildDatadogQueryForExternalMetric(externalMetric.metricName, externalMetric.metricLabels)
			autogenDatadogMetric := model.NewDatadogMetricInternalFromExternalMetric(
				datadogMetricID,
				autogenQuery,
				externalMetric.metricName,
				strings.Join(externalMetric.autoscalerReferences, autoscalerReferencesSep),
			)
			log.Infof("Creating DatadogMetric: %s for ExternalMetric: %s, Query: %s", datadogMetricID, externalMetric.metricName, autogenQuery)
			w.store.Set(datadogMetricID, autogenDatadogMetric, autoscalerWatcherStoreID)
		}
	}
}

func (w *AutoscalerWatcher) updateDatadogMetricStatus(active bool, autoscalerReferences string, datadogMetric model.DatadogMetricInternal) {
	if active != datadogMetric.Active || autoscalerReferences != datadogMetric.AutoscalerReferences {
		log.Debugf("Updating active status for: %s to: %t references: %s", datadogMetric.ID, active, autoscalerReferences)

		if currentDatadogMetric := w.store.LockRead(datadogMetric.ID, false); currentDatadogMetric != nil {
			currentDatadogMetric.UpdateTime = time.Now().UTC()
			currentDatadogMetric.Active = active
			currentDatadogMetric.AutoscalerReferences = autoscalerReferences
			// If we move from Active to Inactive, we discard current valid state to avoid using unrefreshed metrics upon re-activation
			if !currentDatadogMetric.Active {
				currentDatadogMetric.Valid = false
			}

			w.store.UnlockSet(currentDatadogMetric.ID, *currentDatadogMetric, autoscalerWatcherStoreID)
		}
	}
}

func (w *AutoscalerWatcher) cleanupAutogenDatadogMetric(active bool, datadogMetric model.DatadogMetricInternal) {
	if !active && datadogMetric.Autogen && !datadogMetric.HasBeenUpdatedFor(w.autogenExpirationPeriod) {
		log.Infof("Flagging old autogen DatadogMetric: %s for deletion - last update: %v", datadogMetric.ID, datadogMetric.UpdateTime)
		if currentDatadogMetric := w.store.LockRead(datadogMetric.ID, false); currentDatadogMetric != nil {
			currentDatadogMetric.Deleted = true
			w.store.UnlockSet(currentDatadogMetric.ID, *currentDatadogMetric, autoscalerWatcherStoreID)
		}
	}
}

func (w *AutoscalerWatcher) getAutoscalerReferences() (map[string]*externalMetric, error) {
	datadogMetricReferences := make(map[string]*externalMetric, w.store.Count())

	// Helper func to avoid some copy paste between HPA and WPA
	addAutoscalerReference := func(datadogMetricID, autoscalerReference, metricName string, labels map[string]string) {
		if len(datadogMetricID) == 0 {
			datadogMetricName := getAutogenDatadogMetricNameFromLabels(metricName, labels)
			datadogMetricID = w.autogenNamespace + kubernetesNamespaceSep + datadogMetricName
		}

		extMetric, exists := datadogMetricReferences[datadogMetricID]
		if !exists {
			extMetric = &externalMetric{
				metricName:           metricName,
				metricLabels:         labels,
				autoscalerReferences: []string{autoscalerReference},
			}
			datadogMetricReferences[datadogMetricID] = extMetric
		} else {
			extMetric.autoscalerReferences = append(extMetric.autoscalerReferences, autoscalerReference)
		}
	}

	if w.autoscalerLister != nil {
		hpaList, err := w.autoscalerLister.List(labels.Everything())
		if err != nil {
			return nil, fmt.Errorf("Could not list HPAs (to update DatadogMetric active status): %v", err)
		}

		for _, obj := range hpaList {
			w.processHPAReference(addAutoscalerReference, obj)
		}
	}

	if w.wpaLister != nil {
		wpaList, err := w.wpaLister.ByNamespace(metav1.NamespaceAll).List(labels.Everything())
		if err != nil {
			return nil, fmt.Errorf("Could not list WPAs (to update DatadogMetric active status): %v", err)
		}

		for _, wpaObj := range wpaList {
			wpa := &v1alpha1.WatermarkPodAutoscaler{}
			err := apiserver.UnstructuredIntoWPA(wpaObj, wpa)
			if err != nil {
				log.Errorf("Error converting wpa from the cache %v", err)
				continue
			}

			for _, metric := range wpa.Spec.Metrics {
				if metric.External == nil {
					continue
				}

				external := metric.External
				ref := buildAutoscalerReference(autoscalerWPAKindKey, wpa.ObjectMeta)
				ddMetricID, metricName, labels, ok := w.extractAutoscalerReference(external.MetricName, external.MetricSelector)
				if ok {
					addAutoscalerReference(ddMetricID, ref, metricName, labels)
				}
			}
		}
	}

	return datadogMetricReferences, nil
}

type addAutoscalerReferenceFn func(
	datadogMetricID string,
	autoscalerReference string,
	metricName string,
	labels map[string]string,
)

func (w *AutoscalerWatcher) processHPAReference(addAutoscalerReference addAutoscalerReferenceFn, obj runtime.Object) {
	switch hpa := obj.(type) {
	case *autoscalingv2beta1.HorizontalPodAutoscaler:
		w.processHPAv2beta1Reference(addAutoscalerReference, hpa)
	case *autoscalingv2beta2.HorizontalPodAutoscaler:
		w.processHPAv2beta2Reference(addAutoscalerReference, hpa)
	case *autoscalingv2.HorizontalPodAutoscaler:
		w.processHPAv2Reference(addAutoscalerReference, hpa)
	default:
		log.Errorf("object is not a HorizontalPodAutoscaler, %T instead", obj)
	}
}

func (w *AutoscalerWatcher) processHPAv2beta1Reference(addAutoscalerReference addAutoscalerReferenceFn, hpa *autoscalingv2beta1.HorizontalPodAutoscaler) {
	for _, metric := range hpa.Spec.Metrics {
		if metric.Type != autoscalingv2beta1.ExternalMetricSourceType {
			continue
		}

		if metric.External == nil {
			continue
		}

		external := metric.External
		ref := buildAutoscalerReference(autoscalerHPAKindKey, hpa.ObjectMeta)
		ddMetricID, metricName, labels, ok := w.extractAutoscalerReference(external.MetricName, external.MetricSelector)
		if ok {
			addAutoscalerReference(ddMetricID, ref, metricName, labels)
		}
	}
}

func (w *AutoscalerWatcher) processHPAv2beta2Reference(addAutoscalerReference addAutoscalerReferenceFn, hpa *autoscalingv2beta2.HorizontalPodAutoscaler) {
	for _, metric := range hpa.Spec.Metrics {
		if metric.Type != autoscalingv2beta2.ExternalMetricSourceType {
			continue
		}

		if metric.External == nil {
			continue
		}

		external := metric.External
		ref := buildAutoscalerReference(autoscalerHPAKindKey, hpa.ObjectMeta)
		ddMetricID, metricName, labels, ok := w.extractAutoscalerReference(external.Metric.Name, external.Metric.Selector)
		if ok {
			addAutoscalerReference(ddMetricID, ref, metricName, labels)
		}
	}
}

func (w *AutoscalerWatcher) processHPAv2Reference(addAutoscalerReference addAutoscalerReferenceFn, hpa *autoscalingv2.HorizontalPodAutoscaler) {
	for _, metric := range hpa.Spec.Metrics {
		if metric.Type != autoscalingv2.ExternalMetricSourceType {
			continue
		}

		if metric.External == nil {
			continue
		}

		external := metric.External
		ref := buildAutoscalerReference(autoscalerHPAKindKey, hpa.ObjectMeta)
		ddMetricID, metricName, labels, ok := w.extractAutoscalerReference(external.Metric.Name, external.Metric.Selector)
		if ok {
			addAutoscalerReference(ddMetricID, ref, metricName, labels)
		}
	}
}

func (w *AutoscalerWatcher) extractAutoscalerReference(
	externalMetricName string,
	externalMetricSelector *metav1.LabelSelector,
) (
	ddMetricID string,
	metricName string,
	labels map[string]string,
	ok bool,
) {
	ddMetricID, parsed, hasPrefix := metricNameToDatadogMetricID(externalMetricName)
	if parsed {
		return ddMetricID, "", nil, true
	} else if !hasPrefix && w.autogenEnabled {
		// We were not able to parse name as DatadogMetric ID.
		// It will be considered as a normal metricName +
		// labels
		var labels map[string]string
		if externalMetricSelector != nil {
			labels = externalMetricSelector.MatchLabels
		}

		return "", externalMetricName, labels, true
	}

	return "", "", nil, false
}

func buildAutoscalerReference(kind string, obj metav1.ObjectMeta) string {
	return kind + autoscalerReferencesKindSep + obj.Namespace + kubernetesNamespaceSep + obj.Name
}

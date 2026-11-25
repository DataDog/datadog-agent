// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package store implements the store used by the Kubernetes State Core cluster
// check.
package store

import (
	"slices"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
)

// EventNotifier interface for callback notification
// avoid builder import
type EventNotifier interface {
	NotifyStoreEvent(eventType StoreEventType, resourceType string, obj interface{})
}

// MetricsStore implements the k8s.io/client-go/tools/cache.Store
// interface. Instead of storing entire Kubernetes objects, it stores metrics
// generated based on those objects.
type MetricsStore struct {
	mutex sync.RWMutex
	// metrics is a map indexed by Kubernetes object id, containing a slice of
	// metric families, containing a slice of metrics.
	metrics map[types.UID][]DDMetricsFam
	// generateMetricsFunc generates metrics based on a given Kubernetes object
	// and returns them grouped by metric family.
	generateMetricsFunc func(interface{}) []metric.FamilyInterface

	MetricsType     string
	enableCallbacks bool          // flag to enable event callback functionality
	eventNotifier   EventNotifier // callback notifier (builder reference)

	// lastResourceVersion is the last resource version seen by the store,
	// protected by mutex.
	lastResourceVersion string

	// keys shares the label keys between the series of a family, protected by
	// keysMutex rather than mutex, not to block the readers of the metrics.
	keys      keysInterner
	keysMutex sync.Mutex
}

// DDMetric represents the data we care about for a context.
type DDMetric struct {
	Labels Labels
	Val    float64
}

// DDMetricsFam is the representation of a metric family.
type DDMetricsFam struct {
	Type        string
	Name        string
	ListMetrics []DDMetric
}

// NewMetricsStore returns a new MetricsStore.
func NewMetricsStore(generateFunc func(interface{}) []metric.FamilyInterface, mt string) *MetricsStore {
	return &MetricsStore{
		MetricsType:         mt,
		generateMetricsFunc: generateFunc,
		metrics:             map[types.UID][]DDMetricsFam{},
		enableCallbacks:     false,
		keys:                keysInterner{},
	}
}

// EnableCallbacks enables event callback functionality for this store with a notifier
func (s *MetricsStore) EnableCallbacks(notifier EventNotifier) {
	s.enableCallbacks = true
	s.eventNotifier = notifier
}

func (d *DDMetricsFam) extract(f metric.Family) {
	// f.Type is not extracted (value of gauge, counter etc) as we only support gauges.
	d.Name = f.Name
	// Sized once: the slice is kept as long as the object, so growth slack
	// would be kept too.
	d.ListMetrics = make([]DDMetric, 0, len(f.Metrics))
	for _, m := range f.Metrics {
		s := DDMetric{}
		s.Val = m.Value
		if len(m.LabelKeys) != len(m.LabelValues) {
			// TODO test how verbose that could be.
			log.Errorf("Could not retrieve the labels for %s: %v", f.Name, errLabelsLength)
			continue
		}
		// The values are copied: kube-state-metrics allocates the keys and
		// the values of a metric in a single block, which keeping its values
		// would keep whole, while the keys are shared between series. Add
		// interns the keys, and checks the new ones for duplicates.
		s.Labels = Labels{keys: m.LabelKeys, values: slices.Clone(m.LabelValues)}
		d.ListMetrics = append(d.ListMetrics, s)
	}
	if len(d.ListMetrics) < cap(d.ListMetrics) {
		// Some metrics were skipped: don't keep room for them
		d.ListMetrics = slices.Clone(d.ListMetrics)
	}
}

// Add inserts adds to the MetricsStore by calling the metrics generator functions and
// adding the generated metrics to the metrics map that underlies the MetricStore.
// Implementing k8s.io/client-go/tools/cache.Store interface
func (s *MetricsStore) Add(obj interface{}) error {
	o, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	// Notify callbacks if enabled
	if s.enableCallbacks && s.eventNotifier != nil {
		s.eventNotifier.NotifyStoreEvent(EventAdd, s.MetricsType, obj)
	}

	metricsForUID := s.generateMetricsFunc(obj)
	convertedMetricsForUID := make([]DDMetricsFam, len(metricsForUID))
	for i, f := range metricsForUID {
		metricConvertedList := DDMetricsFam{
			// Used to build a map to easily identify the Object associated with the metrics
			Type: s.MetricsType,
		}
		f.Inspect(metricConvertedList.extract)
		convertedMetricsForUID[i] = metricConvertedList
	}
	s.keysMutex.Lock()
	for i := range convertedMetricsForUID {
		family := &convertedMetricsForUID[i]
		for j := range family.ListMetrics {
			labels := &family.ListMetrics[j].Labels
			*labels = s.keys.intern(family.Name, *labels)
		}
	}
	s.keysMutex.Unlock()

	// We need to keep the store with UID as a key to handle the lifecycle of the objects and the metrics attached.
	s.mutex.Lock()
	s.metrics[o.GetUID()] = convertedMetricsForUID
	s.lastResourceVersion = o.GetResourceVersion()
	s.mutex.Unlock()

	return nil
}

// Update updates the existing entry in the MetricsStore by overriding it.
func (s *MetricsStore) Update(obj interface{}) error {
	// Notify callbacks if enabled
	if s.enableCallbacks && s.eventNotifier != nil {
		s.eventNotifier.NotifyStoreEvent(EventUpdate, s.MetricsType, obj)
	}

	// TODO: For now, just call Add, in the future one could check if the resource version changed?
	return s.Add(obj)
}

// Delete deletes an existing entry in the MetricsStore.
func (s *MetricsStore) Delete(obj interface{}) error {
	o, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	// Notify callbacks if enabled
	if s.enableCallbacks && s.eventNotifier != nil {
		s.eventNotifier.NotifyStoreEvent(EventDelete, s.MetricsType, obj)
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	delete(s.metrics, o.GetUID())
	s.lastResourceVersion = o.GetResourceVersion()

	return nil
}

// List implements the List method of the store interface.
func (s *MetricsStore) List() []interface{} {
	return nil
}

// ListKeys implements the ListKeys method of the store interface.
func (s *MetricsStore) ListKeys() []string {
	return nil
}

// Get implements the Get method of the store interface.
func (s *MetricsStore) Get(_ interface{}) (item interface{}, exists bool, err error) {
	return nil, false, nil
}

// GetByKey implements the GetByKey method of the store interface.
func (s *MetricsStore) GetByKey(_ string) (item interface{}, exists bool, err error) {
	return nil, false, nil
}

// Replace will delete the contents of the store, using instead the
// given list, and records the given resource version as the last one seen.
func (s *MetricsStore) Replace(list []interface{}, resourceVersion string) error {
	s.mutex.Lock()
	s.metrics = map[types.UID][]DDMetricsFam{}
	s.mutex.Unlock()

	for _, o := range list {
		err := s.Add(o)
		if err != nil {
			return err
		}
	}

	s.Bookmark(resourceVersion)

	return nil
}

// Resync implements the Resync method of the store interface.
func (s *MetricsStore) Resync() error {
	return nil
}

// Bookmark implements the Bookmark method of the store interface.
func (s *MetricsStore) Bookmark(resourceVersion string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.lastResourceVersion = resourceVersion
}

// LastStoreSyncResourceVersion implements the LastStoreSyncResourceVersion
// method of the store interface.
func (s *MetricsStore) LastStoreSyncResourceVersion() string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.lastResourceVersion
}

// FamilyAllow is a metric-family-based filtering function provided by the store clients
type FamilyAllow func(DDMetricsFam) bool

// GetAllFamilies is family metric filter that allows all metric families
var GetAllFamilies FamilyAllow = func(DDMetricsFam) bool { return true }

// MetricAllow is a metric-based filtering function provided by the store clients
type MetricAllow func(DDMetric) bool

// GetAllMetrics is a metric filter that allows all metrics
var GetAllMetrics MetricAllow = func(DDMetric) bool { return true }

// Push is used to take all the metrics from the store and push them to the check for
// further processing.
// FamilyAllow and MetricAllow filtering functions can be used
// to get a subset of metrics from the store.
//
// The returned metrics can be shared with the store and with the results of
// other calls: callers must not modify them.
func (s *MetricsStore) Push(familyFilter FamilyAllow, metricFilter MetricAllow) map[string][]DDMetricsFam {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// The first pass is to presize the result map and the per-name slices.
	familyCounts := make(map[string]int)
	for _, metricFamList := range s.metrics {
		for _, metricFam := range metricFamList {
			if !familyFilter(metricFam) {
				continue
			}
			if len(metricFam.ListMetrics) == 0 {
				continue
			}
			familyCounts[metricFam.Name]++
		}
	}

	mRes := make(map[string][]DDMetricsFam, len(familyCounts))

	// Iterate through all metrics with filters
	for _, metricFamList := range s.metrics {
		for _, metricFam := range metricFamList {
			if !familyFilter(metricFam) {
				continue
			}

			// Skip families with no metrics - nothing to process
			if len(metricFam.ListMetrics) == 0 {
				continue
			}

			resMetric := filterMetrics(metricFam.ListMetrics, metricFilter)

			// Skip families where all metrics were filtered out
			if len(resMetric) == 0 {
				continue
			}

			fams, found := mRes[metricFam.Name]
			if !found {
				fams = make([]DDMetricsFam, 0, familyCounts[metricFam.Name])
			}
			mRes[metricFam.Name] = append(fams, DDMetricsFam{
				ListMetrics: resMetric,
				Type:        metricFam.Type,
				Name:        metricFam.Name,
			})
		}
	}

	return mRes
}

// filterMetrics returns the metrics that keep allows. It returns metrics itself
// when keep allows all of them: Add replaces the metrics of an object rather
// than modifying them, so the clients can read them after the lock is released,
// as long as they don't modify them.
func filterMetrics(metrics []DDMetric, keep MetricAllow) []DDMetric {
	i := slices.IndexFunc(metrics, func(m DDMetric) bool { return !keep(m) })
	if i < 0 {
		// Clipped, so that an append can't write in the store
		return slices.Clip(metrics)
	}
	res := slices.Clone(metrics[:i])
	for _, metric := range metrics[i+1:] {
		if keep(metric) {
			res = append(res, metric)
		}
	}
	return res
}

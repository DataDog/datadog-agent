// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package store implements the store used by the Kubernetes State Core cluster
// check.
package store

import (
	"errors"
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
	// deletionMarkerFamily, when set, names the metric family whose presence
	// marks an object as being deleted. Objects carrying it are retained in
	// recentlyDeleted when they leave the store. Empty disables retention.
	deletionMarkerFamily string
	// recentlyDeleted retains the metric families of the objects that carried
	// deletionMarkerFamily when they left the store, so that a consumer can
	// emit a final point for an object that appeared and disappeared between
	// two scrapes. Emptied by DrainRecentlyDeleted or ClearRecentlyDeleted.
	recentlyDeleted map[types.UID][]DDMetricsFam
	// generateMetricsFunc generates metrics based on a given Kubernetes object
	// and returns them grouped by metric family.
	generateMetricsFunc func(interface{}) []metric.FamilyInterface

	MetricsType     string
	enableCallbacks bool          // flag to enable event callback functionality
	eventNotifier   EventNotifier // callback notifier (builder reference)
}

// DDMetric represents the data we care about for a context.
type DDMetric struct {
	Labels map[string]string
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
	}
}

// EnableCallbacks enables event callback functionality for this store with a notifier
func (s *MetricsStore) EnableCallbacks(notifier EventNotifier) {
	s.enableCallbacks = true
	s.eventNotifier = notifier
}

// EnableDeletionRetention makes the store retain the metric families of an
// object that carries the markerFamily metric family when that object leaves
// the store. The retained families are read back with DrainRecentlyDeleted.
// The store itself holds no policy: the caller names the family that marks an
// object as being deleted.
func (s *MetricsStore) EnableDeletionRetention(markerFamily string) {
	s.deletionMarkerFamily = markerFamily
}

func (d *DDMetricsFam) extract(f metric.Family) {
	// f.Type is not extracted (value of gauge, counter etc) as we only support gauges.
	d.Name = f.Name
	for _, m := range f.Metrics {
		var err error
		s := DDMetric{}
		s.Val = m.Value
		s.Labels, err = buildTags(m)
		if err != nil {
			// TODO test how verbose that could be.
			log.Errorf("Could not retrieve the labels for %s: %v", f.Name, err)
			continue
		}
		d.ListMetrics = append(d.ListMetrics, s)
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
	// We need to keep the store with UID as a key to handle the lifecycle of the objects and the metrics attached.
	s.mutex.Lock()
	s.metrics[o.GetUID()] = convertedMetricsForUID
	s.mutex.Unlock()

	return nil
}

func buildTags(metrics *metric.Metric) (map[string]string, error) {
	if len(metrics.LabelKeys) != len(metrics.LabelValues) {
		return nil, errors.New("LabelKeys and LabelValues not same size")
	}
	tags := make(map[string]string, len(metrics.LabelValues))
	for i, key := range metrics.LabelKeys {
		tags[key] = metrics.LabelValues[i]
	}
	return tags, nil
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

	uid := o.GetUID()
	s.retainIfMarkedDeleted(uid, s.metrics[uid])
	delete(s.metrics, uid)

	return nil
}

// retainIfMarkedDeleted moves the metric families of one object to
// recentlyDeleted if they contain the configured deletion marker family.
// It must be called with s.mutex held for writing.
func (s *MetricsStore) retainIfMarkedDeleted(uid types.UID, families []DDMetricsFam) {
	if s.deletionMarkerFamily == "" {
		return
	}

	for _, fam := range families {
		if fam.Name == s.deletionMarkerFamily && len(fam.ListMetrics) > 0 {
			if s.recentlyDeleted == nil {
				s.recentlyDeleted = make(map[types.UID][]DDMetricsFam)
			}
			s.recentlyDeleted[uid] = families
			return
		}
	}
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
// given list.
func (s *MetricsStore) Replace(list []interface{}, _ string) error {
	// Objects that leave the store during a relist (e.g. deleted while the
	// watch was disconnected) get no Delete event, so retention has to happen
	// here too. Build the UID set before taking the lock: list is owned by the
	// caller, and for a large store this loop would otherwise block every
	// informer handler.
	var listUIDs map[types.UID]struct{}
	if s.deletionMarkerFamily != "" {
		listUIDs = make(map[types.UID]struct{}, len(list))
		for _, o := range list {
			if acc, err := meta.Accessor(o); err == nil {
				listUIDs[acc.GetUID()] = struct{}{}
			}
		}
	}

	s.mutex.Lock()

	for uid, metrics := range s.metrics {
		if _, ok := listUIDs[uid]; ok {
			continue
		}
		s.retainIfMarkedDeleted(uid, metrics)
	}

	s.metrics = map[types.UID][]DDMetricsFam{}
	s.mutex.Unlock()

	for _, o := range list {
		err := s.Add(o)
		if err != nil {
			return err
		}
	}

	return nil
}

// Resync implements the Resync method of the store interface.
func (s *MetricsStore) Resync() error {
	return nil
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
func (s *MetricsStore) Push(familyFilter FamilyAllow, metricFilter MetricAllow) map[string][]DDMetricsFam {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	mRes := make(map[string][]DDMetricsFam)
	for _, families := range s.metrics {
		appendFilteredFamilies(mRes, families, familyFilter, metricFilter)
	}

	return mRes
}

// FilterFamilies applies the same filtering as Push to the metric families of a
// single object, as returned by DrainRecentlyDeleted, and returns them grouped
// by metric family name.
func FilterFamilies(families []DDMetricsFam, familyFilter FamilyAllow, metricFilter MetricAllow) map[string][]DDMetricsFam {
	mRes := make(map[string][]DDMetricsFam)
	appendFilteredFamilies(mRes, families, familyFilter, metricFilter)

	return mRes
}

// appendFilteredFamilies adds the families of one object that pass both filters
// to dst, grouped by metric family name.
func appendFilteredFamilies(dst map[string][]DDMetricsFam, families []DDMetricsFam, familyFilter FamilyAllow, metricFilter MetricAllow) {
	for _, metricFam := range families {
		if !familyFilter(metricFam) {
			continue
		}

		// Skip families with no metrics - nothing to process
		if len(metricFam.ListMetrics) == 0 {
			continue
		}

		// Preallocate with full capacity to avoid slice growth reallocations
		resMetric := make([]DDMetric, 0, len(metricFam.ListMetrics))

		for _, metric := range metricFam.ListMetrics {
			if !metricFilter(metric) {
				continue
			}
			resMetric = append(resMetric, metric)
		}

		if len(resMetric) > 0 {
			dst[metricFam.Name] = append(dst[metricFam.Name], DDMetricsFam{
				ListMetrics: resMetric,
				Type:        metricFam.Type,
				Name:        metricFam.Name,
			})
		}
	}
}

// DrainRecentlyDeleted returns the metric families of every object retained by
// EnableDeletionRetention since the last drain, and empties the retention map
// in the same critical section. The swap has to be atomic: with a separate read
// and clear, an object deleted in between would be dropped.
//
// The object UID is not exposed: the caller only needs the families of each
// object, grouped per object.
func (s *MetricsStore) DrainRecentlyDeleted() [][]DDMetricsFam {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if len(s.recentlyDeleted) == 0 {
		return nil
	}

	drained := make([][]DDMetricsFam, 0, len(s.recentlyDeleted))
	for _, families := range s.recentlyDeleted {
		drained = append(drained, families)
	}
	s.recentlyDeleted = nil

	return drained
}

// ClearRecentlyDeleted discards the retained metric families without returning
// them. Used when the consumer is not going to emit them (e.g. a standby
// cluster-agent whose informers keep producing Delete events) to bound memory.
func (s *MetricsStore) ClearRecentlyDeleted() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.recentlyDeleted = nil
}

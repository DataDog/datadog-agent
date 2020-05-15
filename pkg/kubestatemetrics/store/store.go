// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package store

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kube-state-metrics/pkg/metric"
)

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

	MetricsType string
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
	}
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

// Implementing k8s.io/client-go/tools/cache.Store interface
// Add inserts adds to the MetricsStore by calling the metrics generator functions and
// adding the generated metrics to the metrics map that underlies the MetricStore.
func (s *MetricsStore) Add(obj interface{}) error {
	o, err := meta.Accessor(obj)
	if err != nil {
		return err
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
		return nil, fmt.Errorf("LabelKeys and LabelValues not same size")
	}
	tags := make(map[string]string, len(metrics.LabelValues))
	for i, key := range metrics.LabelKeys {
		tags[key] = metrics.LabelValues[i]
	}
	return tags, nil
}

// Update updates the existing entry in the MetricsStore by overriding it.
func (s *MetricsStore) Update(obj interface{}) error {
	// TODO: For now, just call Add, in the future one could check if the resource version changed?
	return s.Add(obj)
}

// Delete deletes an existing entry in the MetricsStore.
func (s *MetricsStore) Delete(obj interface{}) error {
	o, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	delete(s.metrics, o.GetUID())

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
func (s *MetricsStore) Get(obj interface{}) (item interface{}, exists bool, err error) {
	return nil, false, nil
}

// GetByKey implements the GetByKey method of the store interface.
func (s *MetricsStore) GetByKey(key string) (item interface{}, exists bool, err error) {
	return nil, false, nil
}

// Replace will delete the contents of the store, using instead the
// given list.
func (s *MetricsStore) Replace(list []interface{}, _ string) error {
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

// TODO store label_to_get values from the label join here to warm the cache.
// Push is used to take all the metrics from the store and push them to the check for
// further processing.
func (s *MetricsStore) Push() map[string][]DDMetricsFam {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	mRes := make(map[string][]DDMetricsFam)

	for u, metricFamList := range s.metrics {
		for _, metricFam := range metricFamList {
			resMetric := []DDMetric{}
			for _, metric := range metricFam.ListMetrics {
				tags := metric.Labels
				tags["uid"] = string(u)
				resMetric = append(resMetric, DDMetric{
					Val:    metric.Val,
					Labels: tags,
				})
			}
			mRes[metricFam.Name] = append(mRes[metricFam.Name], DDMetricsFam{
				ListMetrics: resMetric,
				Type:        metricFam.Type,
				Name:        metricFam.Name,
			})
		}
	}
	return mRes

}

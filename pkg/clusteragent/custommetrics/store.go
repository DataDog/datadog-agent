// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	keyDelimeter = "-"

	storeLastUpdatedAnnotationKey = "custom-metrics.datadoghq.com/last-updated"
)

var (
	externalTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "external_metrics",
			Help: "Number of external metrics tagged.",
		},
		[]string{"valid"},
	)

	errNotInitialized = fmt.Errorf("configmap not initialized")
)

func init() {
	prometheus.MustRegister(externalTotal)
}

// Store is an interface for persistent storage of custom and external metrics.
type Store interface {
	SetExternalMetricValues(map[string]ExternalMetricValue) error

	DeleteExternalMetricValues(*MetricsBundle) error

	ListAllExternalMetricValues() (*MetricsBundle, error)

	GetMetrics() (*MetricsBundle, error)
}

// configMapStore provides persistent storage of custom and external metrics using a configmap.
type configMapStore struct {
	namespace string
	name      string
	client    corev1.CoreV1Interface
	mu        sync.RWMutex
	cm        *v1.ConfigMap
}

// GetConfigmapName returns the name of the ConfigMap used to store the state of the Custom Metrics Provider
func GetConfigmapName() string {
	return config.Datadog.GetString("hpa_configmap_name")
}

// NewConfigMapStore returns a new store backed by a configmap. The configmap will be created
// in the specified namespace if it does not exist.
func NewConfigMapStore(client kubernetes.Interface, ns, name string) (Store, error) {
	store := &configMapStore{
		namespace: ns,
		name:      name,
		client:    client.CoreV1(),
	}
	err := store.getConfigMap()
	if err == nil {
		log.Infof("Retrieved the configmap %s", name)
		return store, nil
	}

	if !errors.IsNotFound(err) {
		log.Infof("Error while attempting to fetch the configmap %s: %v", name, err)
		return nil, err
	}

	log.Infof("The configmap %s does not exist, trying to create it", name)
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
	setLastUpdatedAnnotation(cm)

	// FIXME: distinguish RBAC error
	store.cm, err = client.CoreV1().ConfigMaps(ns).Create(cm)
	if err != nil {
		return nil, err
	}
	return store, nil
}

// SetExternalMetricValues updates the external metrics in the configmap.
func (c *configMapStore) SetExternalMetricValues(added map[string]ExternalMetricValue) error {
	if len(added) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cm == nil {
		return errNotInitialized
	}
	if c.cm.Data == nil {
		// Don't panic "assignment to entry in nil map" at init
		c.cm.Data = make(map[string]string)
	}
	for key, m := range added {
		toStore, err := json.Marshal(m)
		if err != nil {
			log.Debugf("Could not marshal the external metric %v: %v", m, err)
			continue
		}
		c.cm.Data[key] = string(toStore)
	}
	return c.updateConfigMap()
}

// DeleteExternalMetricValues deletes the external metrics from the store.
func (c *configMapStore) DeleteExternalMetricValues(deleted *MetricsBundle) error {
	if len(deleted.External) == 0 && len(deleted.Deprecated) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cm == nil {
		return errNotInitialized
	}
	for _, m := range deleted.External {
		key := ExternalMetricValueKeyFunc(m)
		delete(c.cm.Data, key)
		log.Debugf("Deleted metric %s for Autoscaler %s/%s from the configmap %s", m.MetricName, m.Ref.Namespace, m.Ref.Name, c.name)
	}
	for _, m := range deleted.Deprecated {
		key := DeprecatedExternalMetricValueKeyFunc(m)
		delete(c.cm.Data, key)
		log.Debugf("Deleted key %s deprecated metric %s for HPA %s/%s from the configmap %s", key, m.MetricName, m.HPA.Namespace, m.HPA.Name, c.name)
	}
	return c.updateConfigMap()
}

// ListAllExternalMetricValues returns the most up-to-date list of external metrics from the configmap.
// Any replica can safely call this function.
func (c *configMapStore) ListAllExternalMetricValues() (*MetricsBundle, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.getConfigMap(); err != nil {
		return nil, err
	}
	return c.doGetMetrics()
}

// ListAllDeprecatedMetricValues returns the list of external metrics that do not use the new format.
// Any replica can safely call this function. Although it should only be called to clean the Store.
func (c *configMapStore) ListAllDeprecatedMetricValues() ([]DeprecatedExternalMetricValue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.getConfigMap(); err != nil {
		return nil, err
	}
	bundle, err := c.doGetMetrics()
	if err != nil {
		return nil, err
	}
	return bundle.Deprecated, nil
}

// GetMetrics returns a bundle of all the metrics from the local copy of the configmap.
func (c *configMapStore) GetMetrics() (*MetricsBundle, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.doGetMetrics()
}

func (c *configMapStore) doGetMetrics() (*MetricsBundle, error) {
	bundle := &MetricsBundle{}
	for k, v := range c.cm.Data {
		if !isExternalMetricValueKey(k) {
			continue
		}
		m := ExternalMetricValue{}
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			log.Debugf("Could not unmarshal the external metric for key %s: %v", k, err)
			continue
		}
		if m.Ref.Type == "" {
			// We are processing a deprecated format, invalidate for now.
			deprecated := DeprecatedExternalMetricValue{}
			if err := json.Unmarshal([]byte(v), &deprecated); err != nil {
				log.Debugf("Could not unmarshal the external metric for key %s: %v", k, err)
				continue
			}
			deprecated.Valid = false
			bundle.Deprecated = append(bundle.Deprecated, deprecated)
			continue
		}
		bundle.External = append(bundle.External, m)
	}
	return bundle, nil
}

func (c *configMapStore) getConfigMap() error {
	var err error
	c.cm, err = c.client.ConfigMaps(c.namespace).Get(c.name, metav1.GetOptions{})
	if err != nil {
		log.Infof("Could not get the configmap %s: %v", c.name, err)
		return err
	}
	return nil
}

func (c *configMapStore) updateConfigMap() error {
	setLastUpdatedAnnotation(c.cm)
	var err error
	c.cm, err = c.client.ConfigMaps(c.namespace).Update(c.cm)
	if err != nil {
		log.Infof("Could not update the configmap %s: %v", c.name, err)
		return err
	}
	setStoreStats(c)
	return nil
}

// ExternalMetricValueKeyFunc knows how to make keys for storing external metrics. The key
// is unique for each metric of an Autoscaler. This means that the keys for the same metric from two
// different HPAs will be different (important for external metrics that may use different labels
// for the same metric).
func ExternalMetricValueKeyFunc(val ExternalMetricValue) string {
	parts := []string{
		"external_metric",
		val.Ref.Type,
		val.Ref.Namespace,
		val.Ref.Name,
		val.MetricName,
	}
	return strings.Join(parts, keyDelimeter)
}

func DeprecatedExternalMetricValueKeyFunc(val DeprecatedExternalMetricValue) string {
	parts := []string{
		"external_metric",
		val.HPA.Namespace,
		val.HPA.Name,
		val.MetricName,
	}
	return strings.Join(parts, keyDelimeter)
}

func isExternalMetricValueKey(key string) bool {
	return strings.HasPrefix(key, "external_metric")
}

func setLastUpdatedAnnotation(cm *v1.ConfigMap) {
	if cm.Annotations == nil {
		// Don't panic "assignment to entry in nil map" at init
		cm.Annotations = make(map[string]string)
	}
	cm.Annotations[storeLastUpdatedAnnotationKey] = time.Now().Format(time.RFC3339)
}

func setStoreStats(store *configMapStore) {
	if store.name != GetConfigmapName() {
		return
	}

	bundle, err := store.doGetMetrics()
	if err != nil {
		return
	}

	var valid, invalid float64
	for _, metric := range bundle.External {
		if metric.Valid {
			valid += 1
		} else {
			invalid += 1
		}
	}
	externalTotal.With(prometheus.Labels{"valid": "true"}).Set(valid)
	externalTotal.With(prometheus.Labels{"valid": "false"}).Set(invalid)
}

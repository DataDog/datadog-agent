// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"encoding/json"
	"expvar"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	storeStats    = expvar.NewMap("custommetrics")
	externalStats = new(expvar.Map).Init()
	externalTotal = &expvar.Int{}
	externalValid = &expvar.Int{}

	errNotInitialized = fmt.Errorf("configmap not initialized")
)

func init() {
	storeStats.Set("External", externalStats)
	externalStats.Set("Total", externalTotal)
	externalStats.Set("Valid", externalValid)
}

// Store is an interface for persistent storage of custom and external metrics.
type Store interface {
	Add(*MetricsBundle) error
	Delete(*MetricsBundle) error
	Dump() (*MetricsBundle, error)
	ResyncAndDump() (*MetricsBundle, error)
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

// Add adds metrics in the bundle to the configmap. Only the leader should call this function.
func (c *configMapStore) Add(bundle *MetricsBundle) error {
	if bundle.Len() == 0 {
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
	for _, m := range bundle.External {
		key := externalMetricValueKeyFunc(m)
		toStore, err := json.Marshal(m)
		if err != nil {
			log.Debugf("Could not marshal the external metric %v: %v", m, err)
			continue
		}
		c.cm.Data[key] = string(toStore)
	}
	return c.updateConfigMap()
}

// Delete deletes metrics in the bundle from the configmap. Only the leader should call this function.
func (c *configMapStore) Delete(bundle *MetricsBundle) error {
	if bundle.Len() == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cm == nil {
		return errNotInitialized
	}
	for _, m := range bundle.External {
		key := externalMetricValueKeyFunc(m)
		delete(c.cm.Data, key)
		log.Debugf("Deleted metric %s for HPA %s/%s from the configmap %s", m.MetricName, m.HPA.Namespace, m.HPA.Name, c.name)
	}
	return c.updateConfigMap()
}

// Dump returns a bundle of all metrics in the configmap.
func (c *configMapStore) Dump() (*MetricsBundle, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.doDump()
}

func (c *configMapStore) doDump() (*MetricsBundle, error) {
	if c.cm == nil {
		return nil, errNotInitialized
	}
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
		bundle.External = append(bundle.External, m)
	}
	return bundle, nil
}

// ResyncAndDump syncs the local configmap with the apiserver and returns bundle of all
// metrics in the configmap. Any replica can safely call this function.
func (c *configMapStore) ResyncAndDump() (*MetricsBundle, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.getConfigMap(); err != nil {
		return nil, err
	}
	return c.doDump()
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

// externalMetricValueKeyFunc knows how to make keys for storing external metrics. The key
// is unique for each metric of an HPA. This means that the keys for the same metric from two
// different HPAs will be different (important for external metrics that may use different labels
// for the same metric).
func externalMetricValueKeyFunc(val ExternalMetricValue) string {
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

func setStoreStats(store *configMapStore) {
	bundle, err := store.doDump()
	if err != nil {
		return
	}

	total := int64(len(bundle.External))
	externalTotal.Set(total)

	valid := int64(0)
	for _, metric := range bundle.External {
		if metric.Valid {
			valid += 1
		}
	}
	externalValid.Set(valid)
}

func setLastUpdatedAnnotation(cm *v1.ConfigMap) {
	if cm.Annotations == nil {
		// Don't panic "assignment to entry in nil map" at init
		cm.Annotations = make(map[string]string)
	}
	cm.Annotations[storeLastUpdatedAnnotationKey] = time.Now().Format(time.RFC3339)
}

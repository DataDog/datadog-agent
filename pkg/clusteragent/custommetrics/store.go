// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

// Store is an interface for persistent storage of custom and external metrics.
type Store interface {
	SetExternalMetric(ExternalMetricValue) error
	DeleteExternalMetric(hpaNamespace, hpaName, metricName string) error
	ListAllExternalMetrics() ([]ExternalMetricValue, error)
	Update() error
}

// configMapStore provides persistent storage of custom and external metrics using a configmap.
type configMapStore struct {
	namespace string
	name      string
	client    corev1.CoreV1Interface
	cm        *v1.ConfigMap
}

// NewConfigMapStore returns a new store backed by a configmap. The configmap will be created
// in the specified namespace if it does not exist.
func NewConfigMapStore(client kubernetes.Interface, ns, name string) (Store, error) {
	cm, err := client.CoreV1().ConfigMaps(ns).Get(name, metav1.GetOptions{})
	if err == nil {
		log.Infof("Retrieved the configmap %s", name)
		return &configMapStore{
			namespace: ns,
			name:      name,
			client:    client.CoreV1(),
			cm:        cm,
		}, nil
	}

	if !errors.IsNotFound(err) {
		log.Infof("Error while attempting to fetch the configmap %s: %s", name, err)
		return nil, err
	}

	log.Infof("The configmap %s does not exist, trying to create it", name)
	cm = &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
	// FIXME: distinguish RBAC error
	cm, err = client.CoreV1().ConfigMaps(ns).Create(cm)
	if err != nil {
		return nil, err
	}
	return &configMapStore{
		namespace: ns,
		name:      name,
		client:    client.CoreV1(),
		cm:        cm,
	}, nil
}

// SetExternalMetric updates the associated external metric in the cached configmap.
func (c *configMapStore) SetExternalMetric(em ExternalMetricValue) error {
	if c.cm == nil {
		return fmt.Errorf("configmap not initialized")
	}
	if c.cm.Data == nil {
		// Don't panic "assignment to entry in nil map" at init
		c.cm.Data = make(map[string]string)
	}
	key := fmt.Sprintf("external_metric.%s.%s-%s", em.HPANamespace, em.HPAName, em.MetricName)
	toStore, _ := json.Marshal(em)
	c.cm.Data[key] = string(toStore)
	return nil
}

// DeleteExternalMetric deletes the associated external metric from the cached configmap.
func (c *configMapStore) DeleteExternalMetric(hpaNamespace, hpaName, metricName string) error {
	if c.cm == nil {
		return fmt.Errorf("configmap not initialized")
	}
	key := fmt.Sprintf("external_metric.%s.%s-%s", hpaNamespace, hpaName, metricName)
	if c.cm.Data[key] == "" {
		log.Debugf("No data for external metric %s", metricName)
		return nil
	}
	delete(c.cm.Data, key)
	log.Debugf("Deleted external metric %#v from the configmap %s", metricName, c.name)
	return nil
}

// ListAllExternalMetrics returns the most up-to-date list of external metrics from the configmap.
// Any replica can safely call this function.
func (c *configMapStore) ListAllExternalMetrics() ([]ExternalMetricValue, error) {
	var metrics []ExternalMetricValue
	var err error
	c.cm, err = c.client.ConfigMaps(c.namespace).Get(c.name, metav1.GetOptions{})
	if err != nil {
		log.Infof("Could not get the configmap %s: %s", c.name, err.Error())
		return nil, err
	}
	for _, val := range c.cm.Data {
		em := &ExternalMetricValue{}
		json.Unmarshal([]byte(val), &em)
		metrics = append(metrics, *em)
	}
	return metrics, nil
}

// Update updates the underlying configmap.
func (c *configMapStore) Update() error {
	if c.cm == nil {
		return fmt.Errorf("configmap not initialized")
	}
	var err error
	c.cm, err = c.client.ConfigMaps(c.namespace).Update(c.cm)
	if err != nil {
		log.Infof("Could not update the configmap %s: %s", c.name, err.Error())
		return err
	}
	return nil
}

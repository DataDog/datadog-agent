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
	SetExternalMetrics([]ExternalMetricValue) error
	DeleteExternalMetrics([]ExternalMetricInfo) error
	ListAllExternalMetrics() ([]ExternalMetricValue, error)
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

// SetExternalMetrics updates the external metrics in the configmap.
func (c *configMapStore) SetExternalMetrics(added []ExternalMetricValue) error {
	if c.cm == nil {
		return fmt.Errorf("configmap not initialized")
	}
	if len(added) == 0 {
		return nil
	}
	if c.cm.Data == nil {
		// Don't panic "assignment to entry in nil map" at init
		c.cm.Data = make(map[string]string)
	}
	for _, em := range added {
		key := fmt.Sprintf("external_metric.%s.%s-%s", em.HPANamespace, em.HPAName, em.MetricName)
		toStore, _ := json.Marshal(em)
		c.cm.Data[key] = string(toStore)
	}
	return c.updateConfigMap()
}

// DeleteExternalMetrics deletes the external metric from the configmap associated with the hpas.
func (c *configMapStore) DeleteExternalMetrics(deleted []ExternalMetricInfo) error {
	if c.cm == nil {
		return fmt.Errorf("configmap not initialized")
	}
	if len(deleted) == 0 {
		return nil
	}
	for _, info := range deleted {
		key := fmt.Sprintf("external_metric.%s.%s-%s", info.HPANamespace, info.HPAName, info.MetricName)
		if c.cm.Data[key] == "" {
			log.Debugf("No data for external metric %s", info.MetricName)
			continue
		}
		delete(c.cm.Data, key)
		log.Debugf("Deleted external metric %#v from the configmap %s", info.MetricName, c.name)
	}
	return c.updateConfigMap()
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

func (c *configMapStore) updateConfigMap() error {
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

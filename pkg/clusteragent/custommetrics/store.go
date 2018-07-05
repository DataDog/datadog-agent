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
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type Tx interface {
	Set(ExternalMetricValue)
	Delete(uid string, metricName string)
}

type Store interface {
	Begin(func(Tx)) error
	ListAllExternalMetrics() ([]ExternalMetricValue, error)
}

// configMapStore provides persistent storage for custom and external metrics using a configmap.
type configMapStore struct {
	namespace string
	name      string
	client    corev1.CoreV1Interface
	cm        *v1.ConfigMap
}

// NewConfigMapStore returns a new store backed by a configmap. The configmap will be created
// in the specified namespace if it does not exist.
func NewConfigMapStore(client corev1.CoreV1Interface, ns, name string) (Store, error) {
	cm, err := client.ConfigMaps(ns).Get(name, metav1.GetOptions{})
	if err == nil {
		log.Infof("Retrieved the configmap %s", name)
		return &configMapStore{
			namespace: ns,
			name:      name,
			client:    client,
			cm:        cm,
		}, nil
	}

	if !errors.IsNotFound(err) {
		log.Infof("Error while attempting to fetch the configmap %s: %s", name, err)
		return nil, err
	}

	log.Infof("The configmap %s not dot exist, trying to create it", name)
	cm = &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
	// FIXME: distinguish RBAC error
	cm, err = client.ConfigMaps(ns).Create(cm)
	if err != nil {
		return nil, err
	}
	return &configMapStore{
		namespace: ns,
		name:      name,
		client:    client,
		cm:        cm,
	}, nil
}

// Begin begins a series of updates/deletes on the store. Only the leader replica should
// call this function.
func (c *configMapStore) Begin(f func(Tx)) error {
	if c.cm == nil {
		return fmt.Errorf("configmap not initialized")
	}
	tx := &configMapTx{c.cm}
	f(tx)
	var err error
	c.cm, err = c.client.ConfigMaps(c.namespace).Update(c.cm)
	if err != nil {
		log.Infof("Could not update the configmap %s: %s", c.name, err.Error())
		return err
	}
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
	for _, d := range c.cm.Data {
		cm := &ExternalMetricValue{}
		json.Unmarshal([]byte(d), &cm)
		metrics = append(metrics, *cm)
	}
	return metrics, nil
}

type configMapTx struct {
	cm *v1.ConfigMap
}

func (c *configMapTx) Set(m ExternalMetricValue) {
	key := fmt.Sprintf("external_metric.%s.%s", m.OwnerRef.UID, m.MetricName)
	toStore, _ := json.Marshal(m)
	if c.cm.Data == nil {
		// Don't panic "assignment to entry in nil map" at init
		c.cm.Data = make(map[string]string)
	}
	c.cm.Data[key] = string(toStore)
}

func (c *configMapTx) Del(uid string, metricName string) {
	key := fmt.Sprintf("external_metric.%s.%s", uid, metricName)
	if c.cm.Data[key] == "" {
		log.Debugf("No data for external metric %s", metricName)
		return
	}
	delete(c.cm.Data, key)
	log.Debugf("Removed external metric %#v from the configmap %s", metricName, c.cm.Name)
}

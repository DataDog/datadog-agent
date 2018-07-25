// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"encoding/json"
	"fmt"
	"strings"

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
	numKeyParts  = 5
)

// Store is an interface for persistent storage of custom and external metrics.
type Store interface {
	SetExternalMetricValues([]ExternalMetricValue) error
	SetMetricDescriptors([]PodsMetricDescriptor, []ObjectMetricDescriptor) error

	Purge([]ObjectReference) error

	ListAllExternalMetricValues() ([]ExternalMetricValue, error)
	ListAllMetricDescriptors() ([]PodsMetricDescriptor, []ObjectMetricDescriptor, error)
}

// configMapStore provides persistent storage of custom and external metrics using a configmap.
type configMapStore struct {
	namespace string
	name      string
	client    corev1.CoreV1Interface
	cm        *v1.ConfigMap
}

// GetHPAConfigmapName returns the name of the ConfigMap used to store the state of the Custom Metrics Provider
func GetHPAConfigmapName() string {
	return config.Datadog.GetString("hpa_configmap_name")
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

// SetExternalMetricValues updates the external metrics in the configmap.
func (c *configMapStore) SetExternalMetricValues(added []ExternalMetricValue) error {
	if len(added) == 0 {
		return nil
	}
	var err error
	for _, metric := range added {
		key := strings.Join([]string{"value", "external", metric.HPARef.Namespace, metric.HPARef.Name, metric.MetricName}, keyDelimeter)
		if err = c.set(key, metric); err == nil {
			continue
		}
		log.Debugf("Could not marshal the external metric %v: %s", metric, err)
	}
	return c.updateConfigMap()
}

func (c *configMapStore) SetMetricDescriptors(podsMetrics []PodsMetricDescriptor, objectMetrics []ObjectMetricDescriptor) error {
	if len(podsMetrics) == 0 && len(objectMetrics) == 0 {
		return nil
	}
	var err error
	for _, desc := range podsMetrics {
		key := strings.Join([]string{"descriptor", "pods", desc.HPARef.Namespace, desc.HPARef.Name, desc.MetricName}, keyDelimeter)
		if err = c.set(key, desc); err == nil {
			continue
		}
		log.Debugf("Could not marshal the pods metric descriptor %v: %v", desc, err)
	}
	for _, desc := range objectMetrics {
		key := strings.Join([]string{"descriptor", "object", desc.HPARef.Namespace, desc.HPARef.Name, desc.MetricName}, keyDelimeter)
		if err = c.set(key, desc); err == nil {
			continue
		}
		log.Debugf("Could not marshal the object metric descriptor %v: %v", desc, err)
	}
	return c.updateConfigMap()
}

// Purge deletes all data in the configmap that refers to any of the given object references.
func (c *configMapStore) Purge(deleted []ObjectReference) error {
	if c.cm == nil {
		return fmt.Errorf("configmap not initialized")
	}
	if len(deleted) == 0 {
		return nil
	}
	for _, obj := range deleted {
		// Delete all metrics from the configmap that reference this object.
		for k := range c.cm.Data {
			parts := strings.Split(k, keyDelimeter)
			if len(parts) < numKeyParts {
				log.Debugf("Deleting malformed key %s", k)
				delete(c.cm.Data, k)
				continue
			}
			if parts[2] != obj.Namespace || parts[3] != obj.Name {
				continue
			}
			delete(c.cm.Data, k)
			log.Debugf("Deleted metric %s for HPA %s from the configmap %s", parts[4], obj.Name, c.name)
		}
	}
	return c.updateConfigMap()
}

// ListAllExternalMetricValues returns the most up-to-date list of external metrics from the configmap.
func (c *configMapStore) ListAllExternalMetricValues() (externalMetrics []ExternalMetricValue, err error) {
	if err = c.getConfigMap(); err != nil {
		return
	}
	for k, v := range c.cm.Data {
		parts := strings.Split(k, keyDelimeter)
		if len(parts) < numKeyParts {
			continue
		}
		if parts[0] != "value" && parts[1] != "external" {
			continue
		}
		metric := ExternalMetricValue{}
		if err := json.Unmarshal([]byte(v), &metric); err != nil {
			log.Debugf("Could not unmarshal the external metric for key %s: %v", k, err)
			continue
		}
		externalMetrics = append(externalMetrics, metric)
	}
	return
}

// ListAllMetricDescriptors returns the most up-to-date list of metric descriptors from the configmap.
func (c *configMapStore) ListAllMetricDescriptors() (podsMetrics []PodsMetricDescriptor, objectMetrics []ObjectMetricDescriptor, err error) {
	if err = c.getConfigMap(); err != nil {
		return
	}
	for k, v := range c.cm.Data {
		parts := strings.Split(k, keyDelimeter)
		if len(parts) < numKeyParts {
			continue
		}
		if parts[0] != "descriptor" {
			continue
		}
		switch parts[1] {
		case "pods":
			desc := PodsMetricDescriptor{}
			if err := json.Unmarshal([]byte(v), &desc); err != nil {
				log.Debugf("Could not unmarshal the pods metric descriptor for key %s: %v", k, err)
				continue
			}
			podsMetrics = append(podsMetrics, desc)
		case "object":
			desc := ObjectMetricDescriptor{}
			if err := json.Unmarshal([]byte(v), &desc); err != nil {
				log.Debugf("Could not unmarshal the object metric descriptor for key %s: %v", k, err)
				continue
			}
			objectMetrics = append(objectMetrics, desc)
		}
	}
	return
}

func (c *configMapStore) set(key string, obj interface{}) error {
	if c.cm == nil {
		return fmt.Errorf("configmap not initialized")
	}
	if c.cm.Data == nil {
		// Don't panic "assignment to entry in nil map" at init
		c.cm.Data = make(map[string]string)
	}
	toStore, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	c.cm.Data[key] = string(toStore)
	return nil
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
	if c.cm == nil {
		return fmt.Errorf("configmap not initialized")
	}
	var err error
	c.cm, err = c.client.ConfigMaps(c.namespace).Update(c.cm)
	if err != nil {
		log.Infof("Could not update the configmap %s: %v", c.name, err)
		return err
	}
	return nil
}

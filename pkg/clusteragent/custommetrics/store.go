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
)

var (
	storeStats    = expvar.NewMap("custommetrics")
	externalStats = new(expvar.Map).Init()
	externalTotal = &expvar.Int{}
	externalValid = &expvar.Int{}
)

func init() {
	storeStats.Set("External", externalStats)
	externalStats.Set("Total", externalTotal)
	externalStats.Set("Valid", externalValid)
}

// Store is an interface for persistent storage of custom and external metrics.
type Store interface {
	SetExternalMetricValues([]ExternalMetricValue) error

	Delete([]ObjectReference) error

	ListAllExternalMetricValues() ([]ExternalMetricValue, error)
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
	for _, m := range added {
		key := strings.Join([]string{"external_metric", m.HPA.Namespace, m.HPA.Name, m.MetricName}, keyDelimeter)
		toStore, err := json.Marshal(m)
		if err != nil {
			log.Debugf("Could not marshal the external metric %v: %s", m, err)
			continue
		}
		c.cm.Data[key] = string(toStore)
	}
	if err := c.updateConfigMap(); err != nil {
		return err
	}

	total := int64(len(added))
	externalTotal.Set(total)

	valid := int64(0)
	for _, metric := range added {
		if metric.Valid {
			valid += 1
		}
	}

	externalValid.Set(valid)

	return nil
}

// Delete deletes all metrics in the configmap that refer to any of the given object references.
func (c *configMapStore) Delete(deleted []ObjectReference) error {
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
			if len(parts) < 4 {
				log.Debugf("Deleting malformed key %s", k)
				delete(c.cm.Data, k)
				continue
			}
			if parts[1] != obj.Namespace || parts[2] != obj.Name {
				continue
			}
			delete(c.cm.Data, k)
			log.Debugf("Deleted metric %s for HPA %s from the configmap %s", parts[3], obj.Name, c.name)
		}
	}
	return c.updateConfigMap()
}

// ListAllExternalMetricValues returns the most up-to-date list of external metrics from the configmap.
// Any replica can safely call this function.
func (c *configMapStore) ListAllExternalMetricValues() ([]ExternalMetricValue, error) {
	var metrics []ExternalMetricValue
	if err := c.getConfigMap(); err != nil {
		return nil, err
	}
	for k, v := range c.cm.Data {
		if !strings.HasPrefix(k, "external_metric") {
			continue
		}
		m := ExternalMetricValue{}
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			log.Debugf("Could not unmarshal the external metric for key %s: %s", k, err)
			continue
		}
		metrics = append(metrics, m)
	}
	return metrics, nil
}

func (c *configMapStore) getConfigMap() error {
	var err error
	c.cm, err = c.client.ConfigMaps(c.namespace).Get(c.name, metav1.GetOptions{})
	if err != nil {
		log.Infof("Could not get the configmap %s: %s", c.name, err)
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
		log.Infof("Could not update the configmap %s: %s", c.name, err)
		return err
	}
	return nil
}

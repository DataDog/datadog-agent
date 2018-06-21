// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
	"reflect"
	"time"
)

//// Covered by test/integration/util/kube_apiserver/hpa_test.go

var (
	hpaReadTimeout  = 100 * time.Millisecond
	hpaPollItl      = 10 * time.Second
	hpaRefreshItl   = 20 * time.Second
	expectedHPAType = reflect.TypeOf(v2beta1.HorizontalPodAutoscaler{})
)

const (
	datadogHPAConfigMap = "datadog-hpa"
)

type CustomExternalMetric struct {
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	Timestamp int64             `json:"ts"`
	HpaOrigin string            `json:"origin"`
	Value     int64             `json:"value"`
}

func (c *APIClient) hpaWatcher(res string) (new []*v2beta1.HorizontalPodAutoscaler, modified []*v2beta1.HorizontalPodAutoscaler, deleted []*v2beta1.HorizontalPodAutoscaler, resVer string, err error) {

	apiclient, err := GetAPIClient()
	if err != nil {
		log.Errorf("Error creating Client for the HPA: %s", err.Error())
		return nil, nil, nil, "0", err
	}

	hpaInterface := apiclient.ClientSet.AutoscalingV2beta1()
	metaOptions := metav1.ListOptions{Watch: true, ResourceVersion: res}

	hpaWatcher, err := hpaInterface.HorizontalPodAutoscalers(metav1.NamespaceAll).Watch(metaOptions)
	if err != nil {
		log.Infof("Failed to watch %v: %v", expectedHPAType, err)
	}
	defer hpaWatcher.Stop()

	watcherTimeout := time.NewTimer(hpaReadTimeout)
	for {
		select {
		case rcvdHPA, ok := <-hpaWatcher.ResultChan():
			if !ok {
				log.Debugf("Unexpected watch close")
				return nil, nil, nil, "0", err
			}
			currHPA, ok := rcvdHPA.Object.(*v2beta1.HorizontalPodAutoscaler)
			if !ok {
				log.Infof("Wrong type: %s", currHPA)
				continue
			}
			if currHPA.ResourceVersion != "" && currHPA.ResourceVersion != resVer {
				resVer = currHPA.ResourceVersion
			}
			if rcvdHPA.Type == watch.Error {
				log.Infof("Error in the watcher, evaluating: %s", currHPA)
				//status, ok := rcvdHPA.Object.()
			}
			if rcvdHPA.Type == watch.Added {
				log.Infof("Adding this manifest: %s", currHPA)
				new = append(new, currHPA)
			}
			if rcvdHPA.Type == watch.Modified {
				log.Infof("Modifying this manifest: %s", currHPA)
				modified = append(modified, currHPA)
			}
			if rcvdHPA.Type == watch.Deleted {
				deleted = append(deleted, currHPA)
			}

			watcherTimeout.Reset(hpaReadTimeout)
		case <-watcherTimeout.C:
			return new, modified, deleted, resVer, nil
		}
	}
}

// HPAWatcher is ...
func (c *APIClient) HPAWatcher() {
	log.Info("Starting HPA Process ...") // REMOVE
	tickerHPAWatchProcess := time.NewTicker(hpaPollItl)
	tickerHPARefreshProcess := time.NewTicker(hpaRefreshItl)

	var resversion string
	err := c.createConfigMapHPA()
	if err != nil {
		log.Errorf("Could not create the ConfigMap %s to run the HPA process, stopping it: %s", err.Error())
		return
	}
	go func() {
		for {
			select {
			case <-tickerHPAWatchProcess.C:
				newHPA, modified, deleted, res, err := c.hpaWatcher(resversion)
				if err != nil {
					return
				}
				if res != resversion && res != "" {
					log.Infof("res is now %s and resversion is %s", res, resversion) // Investigate this
					resversion = res
					if len(newHPA) > 0 {
						c.storeHPA(newHPA)
					}
					if len(modified) > 0 {
						c.storeHPA(modified)
					}
					if len(deleted) > 0 {
						c.removeEntryFromConfigMap(deleted)
					}
				}
			case <-tickerHPARefreshProcess.C:
				// Update values in configmap
				c.updateCustomMetrics()
			}
		}
	}()
}

func (c *APIClient) updateCustomMetrics() error {
	log.Infof("updating CM in the datadog-hpa")
	namespace := GetResourcesNamespace()
	configMap, err := c.Client.ConfigMaps(namespace).Get(datadogHPAConfigMap, metav1.GetOptions{})
	if err != nil {
		log.Infof("Error while retrieving the Config Map %s", datadogHPAConfigMap)
		return nil
	}
	log.Infof("Retrieving the Config Map %s", datadogHPAConfigMap)
	data := configMap.Data
	log.Infof("we got: %#v", data)
	log.Infof("metanow is %#v", metav1.Now().Unix())
	for name, d := range data {
		cm := &CustomExternalMetric{}

		json.Unmarshal([]byte(d), &cm)
		log.Infof("cm is %#v", cm)
		log.Infof("ts is %#v", cm.Timestamp)

		if metav1.Now().Unix()-cm.Timestamp > 60 { // Configurable expire ?
			log.Infof("name: %#v and data %#v has expired", name, d) //REMOVE
			cm.Timestamp = metav1.Now().Unix()
			cm.Value, err = QueryDatadogExtra(cm.Name, cm.Labels) // check err && can we use cm.Name
			log.Infof("cm after update is %#v", cm)

			if err != nil {
				log.Infof("err querying DD %s", err)
				continue
			}
			c, err := json.Marshal(cm)
			if err != nil {
				log.Infof("err marshalling %s", err)
			}
			data[cm.Name] = string(c)

			log.Infof("updated cm is: %#v", cm)
		}

	}
	_, err = c.Client.ConfigMaps(namespace).Update(configMap)
	if err != nil {
		log.Infof("Could not update because: %s", err)
		return err
	}
	return nil
}

func (c *APIClient) createConfigMapHPA() error {
	namespace := GetResourcesNamespace()
	_, err := c.Client.ConfigMaps(namespace).Get(datadogHPAConfigMap, metav1.GetOptions{})
	// There is an error if it does not exist so we attempt to create it. FIXME distinguish RBAC error
	if err == nil {
		log.Infof("Retrieving the Config Map %s", datadogHPAConfigMap)
		return nil
	}
	log.Infof("Could not get the Config Map to run the HPA, trying to create it: %s", err.Error())
	_, err = c.Client.ConfigMaps(namespace).Create(&v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind: "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      datadogHPAConfigMap,
			Namespace: namespace,
		},
		Data: make(map[string]string), // Confirm this works
	})
	return err

}
func processHPA(list []*v2beta1.HorizontalPodAutoscaler) []CustomExternalMetric {
	// transform HPA into structs to be stored
	var cmList []CustomExternalMetric
	for _, e := range list {
		for _, m := range e.Spec.Metrics {
			var cm CustomExternalMetric
			cm.Name = m.External.MetricName
			cm.Timestamp = metav1.Now().Unix()
			cm.Labels = m.External.MetricSelector.MatchLabels
			cm.HpaOrigin = e.Name
			err := cm.validateMetric()
			if err != nil {
				continue
			}
			log.Infof("Finished processing %#v", cm)
			cmList = append(cmList, cm)
		}
	}
	return cmList
}

func (cm *CustomExternalMetric) validateMetric() error {
	var err error
	cm.Value, err = QueryDatadogExtra(cm.Name, cm.Labels)
	if err != nil {
		log.Infof("Not able to validate %s, because: %s", cm, err.Error())
		return err // TODO
	}
	return nil
}

func (c *APIClient) storeHPA(hpaList []*v2beta1.HorizontalPodAutoscaler) error {
	listCustomMetrics := processHPA(hpaList)

	namespace := GetResourcesNamespace()
	cm, err := c.Client.ConfigMaps(namespace).Get(datadogHPAConfigMap, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Could not store the custom metrics data in the ConfiogMap %s: %s", datadogHPAConfigMap, err.Error())
		return err
	}
	for _, n := range listCustomMetrics {
		newMetric := &CustomExternalMetric{
			Name:      n.Name,
			Labels:    n.Labels,
			HpaOrigin: n.HpaOrigin,
			Value:     n.Value,
			Timestamp: n.Timestamp,
		}
		toStore, _ := json.Marshal(newMetric)

		log.Infof("adding %#v, storing %#v", n, toStore)
		cm.Data[n.Name] = string(toStore)
	}

	_, err = c.Client.ConfigMaps(namespace).Update(cm)
	if err != nil {
		log.Infof("Could not update because: %s", err)
	}
	return nil
}

func (c *APIClient) removeEntryFromConfigMap(deleted []*v2beta1.HorizontalPodAutoscaler) error {
	namespace := GetResourcesNamespace()
	deletedCustomMetrics := processHPA(deleted)
	cm, err := c.Client.ConfigMaps(namespace).Get(datadogHPAConfigMap, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Could not store the custom metrics data in the ConfiogMap %s: %s", datadogHPAConfigMap, err.Error())
		return err
	}
	for _, d := range deletedCustomMetrics {
		if cm.Data[d.Name] != "" {
			delete(cm.Data, cm.Data[d.Name]) // FIXME
		}
	}
	_, err = c.Client.ConfigMaps(namespace).Update(cm)
	if err != nil {
		log.Infof("Could not update because: %s", err) // FIXME
	}
	return nil
	// Remove entry from ConfigMap
}

func (c *APIClient) ReadConfigMap() []CustomExternalMetric {
	// Call in provider, just read the metric keys
	namespace := GetResourcesNamespace()
	var list []CustomExternalMetric
	cm, err := c.Client.ConfigMaps(namespace).Get(datadogHPAConfigMap, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Could not store the custom metrics data in the ConfiogMap %s: %s", datadogHPAConfigMap, err.Error())
		return nil
	}
	data := cm.Data
	for _, d := range data {
		cm := &CustomExternalMetric{}
		json.Unmarshal([]byte(d), &cm)
		log.Infof("Appending cm %#v, *cm %#v to list %#v", cm, *cm, list)
		list = append(list, *cm)
	}
	return list
}

// We need to make sure only the leader writes into the CM, and also checks
// That the metrics in the CM are validated in Datadog (so we can query DD and put the value directly).

// Distinguish the custom metrics and external metrics.

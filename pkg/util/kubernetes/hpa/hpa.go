// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"encoding/json"
	"reflect"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	as "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
)

//// Covered by test/integration/util/kube_apiserver/hpa_test.go

var (
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

// HPAWatcherClient embeds the API Server client and the configuration to refresh metrics from Datadog and watch the HPA Objects' activities
type HPAWatcherClient struct {
	apiClient         *as.APIClient
	hpaReadTimeout    time.Duration
	hpaRefreshItl     time.Duration
	hpaPollItl        time.Duration
	hpaExternalMaxAge time.Duration
}

func (c *HPAWatcherClient) hpaWatcher(res string) (new []*v2beta1.HorizontalPodAutoscaler, modified []*v2beta1.HorizontalPodAutoscaler, deleted []*v2beta1.HorizontalPodAutoscaler, resVer string, err error) {
	hpaInterface := c.apiClient.ClientSet.AutoscalingV2beta1()
	metaOptions := metav1.ListOptions{Watch: true, ResourceVersion: res}

	hpaWatcher, err := hpaInterface.HorizontalPodAutoscalers(metav1.NamespaceAll).Watch(metaOptions)
	if err != nil {
		log.Infof("Failed to watch %v: %v", expectedHPAType, err)
	}
	defer hpaWatcher.Stop()

	watcherTimeout := time.NewTimer(c.hpaReadTimeout)
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

			watcherTimeout.Reset(c.hpaReadTimeout)
		case <-watcherTimeout.C:
			return new, modified, deleted, resVer, nil
		}
	}
}

func newHPAWatcher() *HPAWatcherClient {
	clientAPI, err := as.GetAPIClient()
	if err != nil {
		log.Errorf("Error creating Client for the HPA: %s", err.Error())
		return nil
	}
	hpaPollItl := config.Datadog.GetInt("hpa_watcher_polling_freq")
	hpaRefreshItl := config.Datadog.GetInt("hpa_external_metrics_polling_freq")
	hpaExternalMaxAge := config.Datadog.GetInt("hpa_external_metrics_max_age")
	return &HPAWatcherClient{
		apiClient:         clientAPI,
		hpaReadTimeout:    100 * time.Millisecond,
		hpaPollItl:        time.Duration(hpaPollItl) * time.Second,
		hpaRefreshItl:     time.Duration(hpaRefreshItl) * time.Second,
		hpaExternalMaxAge: time.Duration(hpaExternalMaxAge) * time.Second,
	}
}

// GetHPAWatcherClient returns the HPAWatcherClient
func GetHPAWatcherClient() *HPAWatcherClient {
	return newHPAWatcher()
}

// HPAWatcher runs a watch process of the various HPA objects' activities to process and store the relevant info.
// Refreshes the custom metrics stored as well.
func (c *HPAWatcherClient) HPAWatcher() {
	log.Info("Starting HPA Process ...")
	tickerHPAWatchProcess := time.NewTicker(c.hpaPollItl)
	tickerHPARefreshProcess := time.NewTicker(c.hpaRefreshItl)

	var resversion string
	err := c.createConfigMapHPA()
	if err != nil {
		log.Errorf("Could not create the ConfigMap %s to run the HPA process, stopping: %s", datadogHPAConfigMap, err.Error())
		return
	}

	// Creating a leader election engine to make sure only the leader writes the metrics in the configmap and queries Datadog.
	leaderEngine, err := leaderelection.GetLeaderEngine()
	if err != nil {
		log.Errorf("Could not ensure the leader election is running properly: %s", err)
		return
	}
	leaderEngine.EnsureLeaderElectionRuns()

	go func() {
		for {
			select {
			// Ticker for the HPA Object watcher
			case <-tickerHPAWatchProcess.C:
				if !leaderEngine.IsLeader() {
					continue
				}
				newHPA, modified, deleted, res, err := c.hpaWatcher(resversion)
				if err != nil {
					log.Errorf("Error while watching HPA Objects' activities: %s", err)
					return
				}
				if res != resversion && res != "" {
					resversion = res
					if len(newHPA) > 0 {
						c.storeHPA(newHPA)
					}
					if len(modified) > 0 {
						c.storeHPA(modified)
					}
					if len(deleted) > 0 {
						log.Infof("deleting if resver is diff")
						c.removeEntryFromConfigMap(deleted)
					}
				}
			// Ticker to run the refresh process for the stored external metrics
			case <-tickerHPARefreshProcess.C:
				if !leaderEngine.IsLeader() {
					continue
				}
				c.updateCustomMetrics()
			}
		}
	}()
}

func (c *HPAWatcherClient) updateCustomMetrics() error {
	namespace := as.GetResourcesNamespace()
	maxAge := int64(c.hpaExternalMaxAge.Seconds())
	configMap, err := c.apiClient.Client.ConfigMaps(namespace).Get(datadogHPAConfigMap, metav1.GetOptions{})
	if err != nil {
		log.Infof("Error while retrieving the Config Map %s", datadogHPAConfigMap)
		return nil
	}
	data := configMap.Data
	if len(data) == 0 {
		log.Debugf("No External Metrics to evaluate at the moment")
		return nil
	}
	for _, d := range data {
		cm := &CustomExternalMetric{}

		json.Unmarshal([]byte(d), &cm)

		if metav1.Now().Unix()-cm.Timestamp > maxAge {

			cm.Timestamp = metav1.Now().Unix()
			cm.Value, err = QueryDatadogExtra(cm.Name, cm.Labels)
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
	_, err = c.apiClient.Client.ConfigMaps(namespace).Update(configMap)
	if err != nil {
		log.Infof("Could not update because: %s", err)
		return err
	}
	return nil
}

func (c *HPAWatcherClient) createConfigMapHPA() error {
	namespace := as.GetResourcesNamespace()
	_, err := c.apiClient.Client.ConfigMaps(namespace).Get(datadogHPAConfigMap, metav1.GetOptions{})
	// There is an error if the ConfigMap does not exist so we attempt to create it.
	if err == nil {
		log.Infof("Retrieving the Config Map %s", datadogHPAConfigMap)
		return nil
	}
	log.Infof("Could not get the Config Map to run the HPA, trying to create it: %s", err.Error())

	cm := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind: "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      datadogHPAConfigMap,
			Namespace: namespace,
		},
	}
	// FIXME: distinguish RBAC error
	_, err = c.apiClient.Client.ConfigMaps(namespace).Create(cm)

	return err

}

// processHPA transforms HPA data into structures to be stored upon validation that they are available in Datadog
// TODO Distinguish custom and external
func processHPA(list []*v2beta1.HorizontalPodAutoscaler) []CustomExternalMetric {
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

// validateMetric queries Datadog to validate the availability of a metric
func (cm *CustomExternalMetric) validateMetric() error {
	var err error
	cm.Value, err = QueryDatadogExtra(cm.Name, cm.Labels)
	if err != nil {
		log.Errorf("Not able to validate %s: %s", cm, err.Error())
		return err
	}
	return nil
}

// storeHPA processes the data collected from the HPA object watch to be validated and stored in the datadogHPAConfigMap ConfigMap.
func (c *HPAWatcherClient) storeHPA(hpaList []*v2beta1.HorizontalPodAutoscaler) error {
	listCustomMetrics := processHPA(hpaList)
	namespace := as.GetResourcesNamespace()
	cm, err := c.apiClient.Client.ConfigMaps(namespace).Get(datadogHPAConfigMap, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Could not store the custom metrics data in the ConfigMap %s: %s", datadogHPAConfigMap, err.Error())
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
		if cm.Data == nil {
			// Don't panic "assignment to entry in nil map" at init
			cm.Data = make(map[string]string)
		}
		cm.Data[n.Name] = string(toStore)
	}

	_, err = c.apiClient.Client.ConfigMaps(namespace).Update(cm)
	if err != nil {
		log.Infof("Could not update the ConfigMap: %s", err)
	}
	return err
}

// removeEntryFromConfigMap will remove an External Custom Metric from removeEntryFromConfigMap if the corresponding HPA manifest is deleted.
func (c *HPAWatcherClient) removeEntryFromConfigMap(deleted []*v2beta1.HorizontalPodAutoscaler) error {
	namespace := as.GetResourcesNamespace()
	cm, err := c.apiClient.Client.ConfigMaps(namespace).Get(datadogHPAConfigMap, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Could not store the custom metrics data in the ConfiogMap %s: %s", datadogHPAConfigMap, err.Error())
		return err
	}
	for _, d := range deleted {
		for _, m := range d.Spec.Metrics {
			metricName := m.External.MetricName
			if cm.Data[metricName] != "" {
				delete(cm.Data, metricName)
				log.Debugf("Removed entry %#v from the ConfigMap %s", metricName, datadogHPAConfigMap)
			}
		}
	}
	_, err = c.apiClient.Client.ConfigMaps(namespace).Update(cm)
	if err != nil {
		log.Infof("Could not update because: %s", err)
	}
	return err
}

// ReadConfigMap is used by any replica of the DCA serving the request by Kubernetes to ListExternalMetrics.
// Called every 30 seconds (configurable on Kubernetes's end) by default.
// Just list the content of the ConfigMap `datadogHPAConfigMap`.
func (c *HPAWatcherClient) ReadConfigMap() []CustomExternalMetric {
	namespace := as.GetResourcesNamespace()
	var list []CustomExternalMetric
	cm, err := c.apiClient.Client.ConfigMaps(namespace).Get(datadogHPAConfigMap, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Could not store the custom metrics data in the ConfiogMap %s: %s", datadogHPAConfigMap, err.Error())
		return nil
	}
	data := cm.Data
	for _, d := range data {
		cm := &CustomExternalMetric{}
		json.Unmarshal([]byte(d), &cm)
		list = append(list, *cm)
	}
	return list
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/zorkian/go-datadog-api.v2"
	"k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/core/v1"
	apiMachineryErr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/config"
	as "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	expectedHPAType = reflect.TypeOf(v2beta1.HorizontalPodAutoscaler{})
)

const (
	datadogHPAConfigMap = "datadog-hpa"
)

type CustomExternalMetric struct {
	Name         string            `json:"name"`
	Labels       map[string]string `json:"labels"`
	Timestamp    int64             `json:"ts"`
	HPAName      string            `json:"hpa_name"`
	HPANamespace string            `json:"hpa_namespace"`
	Value        int64             `json:"value"`
	Valid        bool              `json:"valid"`
}

// HPAWatcherClient embeds the API Server client and the configuration to refresh metrics from Datadog and watch the HPA Objects' activities
type HPAWatcherClient struct {
	clientSet      kubernetes.Interface
	readTimeout    time.Duration
	refreshItl     *time.Ticker
	pollItl        *time.Ticker
	externalMaxAge time.Duration
	ns             string
	datadogClient  *datadog.Client
}

func (c *HPAWatcherClient) run(res string) (new []*v2beta1.HorizontalPodAutoscaler, modified []*v2beta1.HorizontalPodAutoscaler, deleted []*v2beta1.HorizontalPodAutoscaler, resVer string, err error) {
	hpaInterface := c.clientSet.AutoscalingV2beta1()

	metaOptions := metav1.ListOptions{Watch: true, ResourceVersion: res}

	hpaWatcher, err := hpaInterface.HorizontalPodAutoscalers(metav1.NamespaceAll).Watch(metaOptions)
	if err != nil {
		log.Infof("Failed to watch %v: %v", expectedHPAType, err)
	}
	defer hpaWatcher.Stop()

	watcherTimeout := time.NewTimer(c.readTimeout)
	for {
		select {
		case rcvdHPA, ok := <-hpaWatcher.ResultChan():
			if !ok {
				log.Debugf("Unexpected watch close")
				return nil, nil, nil, "0", err
			}
			currHPA, ok := rcvdHPA.Object.(*v2beta1.HorizontalPodAutoscaler)
			if !ok {
				log.Errorf("Wrong type: %s", currHPA)
				continue
			}
			if currHPA.ResourceVersion != "" && currHPA.ResourceVersion != resVer {
				resVer = currHPA.ResourceVersion
			}
			if rcvdHPA.Type == watch.Error {
				status, ok := rcvdHPA.Object.(*metav1.Status)
				if !ok {
					return nil, nil, nil, "0", errors.Errorf("error in the watcher, evaluating: %s", currHPA)
				}
				log.Infof("Error while processing the HPA watch: %#v", status)
				continue
			}
			if rcvdHPA.Type == watch.Added {
				log.Debugf("Adding this manifest: %s", currHPA)
				new = append(new, currHPA)
			}
			if rcvdHPA.Type == watch.Modified {
				log.Debugf("Modifying this manifest: %s", currHPA)
				modified = append(modified, currHPA)
			}
			if rcvdHPA.Type == watch.Deleted {
				deleted = append(deleted, currHPA)
			}

			watcherTimeout.Reset(c.readTimeout)
		case <-watcherTimeout.C:
			return new, modified, deleted, resVer, nil
		}
	}
}

// NewHPAWatcherClient returns the HPAWatcherClient
func NewHPAWatcherClient() (*HPAWatcherClient, error) {
	namespace := as.GetResourcesNamespace()
	clientAPI, err := as.GetAPIClient()
	datadogCl, err := NewDatadogClient()
	if err != nil {
		return nil, err
	}
	if err != nil {
		log.Errorf("Error creating Client for the HPA: %s", err.Error())
		return nil, err
	}
	pollInterval := config.Datadog.GetInt("hpa_watcher_polling_freq")
	refreshInterval := config.Datadog.GetInt("hpa_external_metrics_polling_freq")
	externalMaxAge := config.Datadog.GetInt("hpa_external_metrics_max_age")
	return &HPAWatcherClient{
		clientSet:      clientAPI.Cl,
		readTimeout:    100 * time.Millisecond,
		pollItl:        time.NewTicker(time.Duration(pollInterval) * time.Second),
		refreshItl:     time.NewTicker(time.Duration(refreshInterval) * time.Second),
		externalMaxAge: time.Duration(externalMaxAge) * time.Second,
		ns:             namespace,
		datadogClient:  datadogCl,
	}, nil
}

// Start runs a watch process of the various HPA objects' activities to process and store the relevant info.
// Refreshes the custom metrics stored as well.
func (c *HPAWatcherClient) Start() {
	log.Info("Starting HPA Process ...")
	tickerHPAWatchProcess := c.pollItl
	tickerHPARefreshProcess := c.refreshItl

	var resversion string
	err := c.createConfigMapHPA()
	if err != nil {
		log.Errorf("Could not retrieve the ConfigMap %s to run the HPA process, stopping: %s", datadogHPAConfigMap, err.Error())
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
				newHPA, modified, deleted, res, err := c.run(resversion)
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
						c.removeEntryFromConfigMap(deleted)
					}
				}
			// Ticker to run the refresh process for the stored external metrics
			case <-tickerHPARefreshProcess.C:
				if !leaderEngine.IsLeader() {
					continue
				}
				// Updating the metrics against Datadog should not affect the HPA pipeline.
				// If metrics are temporarily unavailable for too long, they will become `Valid=false` and won't be evaluated.
				c.updateCustomMetrics()
			}
		}
	}()
}

func (c *HPAWatcherClient) updateCustomMetrics() {
	maxAge := int64(c.externalMaxAge.Seconds())
	configMap, err := c.clientSet.CoreV1().ConfigMaps(c.ns).Get(datadogHPAConfigMap, metav1.GetOptions{})
	if err != nil {
		log.Infof("Error while retrieving the Config Map %s", datadogHPAConfigMap)
		return
	}

	data := configMap.Data
	if len(data) == 0 {
		log.Debugf("No External Metrics to evaluate at the moment")
		return
	}

	for _, d := range data {
		cm := CustomExternalMetric{}
		err := json.Unmarshal([]byte(d), &cm)
		if err != nil {
			log.Errorf("Could not unmarshal %#v", d)
			continue
		}
		if metav1.Now().Unix()-cm.Timestamp > maxAge || !cm.Valid {
			cm.Valid = false
			cm.Timestamp = metav1.Now().Unix()
			cm.Value, cm.Valid, err = c.validate(cm)
			if err != nil {
				log.Debugf("Could not update the metric %s from Datadog: %s", cm.Name, err.Error())
				continue
			}

			c, err := json.Marshal(cm)
			if err != nil {
				log.Errorf("Error while marshalling the custom metric %s: %s", cm.Name, err.Error())
				continue
			}
			key := fmt.Sprintf("external.metrics.%s.%s-%s", cm.HPANamespace, cm.HPAName, cm.Name)
			data[key] = string(c)
			log.Debugf("Updated the custom metric %#v", cm)
		}

	}
	_, err = c.clientSet.CoreV1().ConfigMaps(c.ns).Update(configMap)
	if err != nil {
		log.Errorf("Could not update because: %s", err)
	}
}

func (c *HPAWatcherClient) createConfigMapHPA() error {
	_, err := c.clientSet.CoreV1().ConfigMaps(c.ns).Get(datadogHPAConfigMap, metav1.GetOptions{})
	// There is an error if the ConfigMap does not exist so we attempt to create it.
	if err == nil {
		log.Infof("Retrieving the Config Map %s", datadogHPAConfigMap)
		return nil
	}
	// We do not need to attempt to create the Config Map if we face an error when trying to get it that is different from "not found"
	if !apiMachineryErr.IsNotFound(err) {
		log.Infof("Error while attempting to fetch the Config Map %s: %s", datadogHPAConfigMap, err.Error())
		return err
	}

	log.Infof("Could not get the Config Map to run the HPA, trying to create it: %s", err.Error())
	cm := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind: "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      datadogHPAConfigMap,
			Namespace: c.ns,
		},
	}
	// FIXME: distinguish RBAC error
	_, err = c.clientSet.CoreV1().ConfigMaps(c.ns).Create(cm)
	return err
}

// processHPA transforms HPA data into structures to be stored upon validation that they are available in Datadog
// TODO Distinguish custom and external
func (hpa *HPAWatcherClient) processHPA(list []*v2beta1.HorizontalPodAutoscaler) []CustomExternalMetric {
	var cmList []CustomExternalMetric
	var err error
	for _, e := range list {
		for _, m := range e.Spec.Metrics {
			var cm CustomExternalMetric
			cm.Name = m.External.MetricName
			cm.Timestamp = metav1.Now().Unix()
			cm.Labels = m.External.MetricSelector.MatchLabels
			cm.HPAName = e.Name
			cm.HPANamespace = e.Namespace
			cm.Value, cm.Valid, err = hpa.validate(cm)
			if err != nil {
				log.Debugf("Not able to process %#v: %s", cm, err)
			}
			cmList = append(cmList, cm)
		}
	}
	return cmList
}

// validate queries Datadog to validate the availability of a metric
func (hpa *HPAWatcherClient) validate(cm CustomExternalMetric) (value int64, valid bool, err error) {
	val, err := hpa.queryDatadogExternal(cm.Name, cm.Labels)
	if err != nil {
		return cm.Value, false, err
	}
	return val, true, nil
}

// storeHPA processes the data collected from the HPA object watch to be validated and stored in the datadogHPAConfigMap ConfigMap.
func (c *HPAWatcherClient) storeHPA(hpaList []*v2beta1.HorizontalPodAutoscaler) error {
	listCustomMetrics := c.processHPA(hpaList)
	cm, err := c.clientSet.CoreV1().ConfigMaps(c.ns).Get(datadogHPAConfigMap, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Could not store the custom metrics data in the ConfigMap %s: %s", datadogHPAConfigMap, err.Error())
		return err
	}
	for _, n := range listCustomMetrics {
		toStore, _ := json.Marshal(n)
		if cm.Data == nil {
			// Don't panic "assignment to entry in nil map" at init
			cm.Data = make(map[string]string)
		}
		// We use a specific key to avoid conflicting when processing several HPA manifests with the same name but in different namespaces.
		key := fmt.Sprintf("external.metrics.%s.%s-%s", n.HPANamespace, n.HPAName, n.Name)
		cm.Data[key] = string(toStore)
	}

	_, err = c.clientSet.CoreV1().ConfigMaps(c.ns).Update(cm)
	if err != nil {
		log.Infof("Could not update the ConfigMap: %s", err)
	}
	return err
}

// removeEntryFromConfigMap will remove an External Custom Metric from removeEntryFromConfigMap if the corresponding HPA manifest is deleted.
func (c *HPAWatcherClient) removeEntryFromConfigMap(deleted []*v2beta1.HorizontalPodAutoscaler) error {
	cm, err := c.clientSet.CoreV1().ConfigMaps(c.ns).Get(datadogHPAConfigMap, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Could not remove the custom metrics data in the ConfigMap %s: %s", datadogHPAConfigMap, err.Error())
		return err
	}
	for _, d := range deleted {
		for _, m := range d.Spec.Metrics {
			metricName := m.External.MetricName
			key := fmt.Sprintf("external.metrics.%s.%s-%s", d.Namespace, d.Name, metricName)
			if cm.Data[key] != "" {
				delete(cm.Data, key)
				log.Debugf("Removed entry %#v from the ConfigMap %s", key, datadogHPAConfigMap)
			}
		}
	}
	_, err = c.clientSet.CoreV1().ConfigMaps(c.ns).Update(cm)
	if err != nil {
		log.Infof("Could not update because: %s", err)
	}
	return err
}

// ReadConfigMap is used by any replica of the DCA serving the request by Kubernetes to ListExternalMetrics.
// Called every 30 seconds (configurable on Kubernetes's end) by default.
// Just list the content of the ConfigMap `datadogHPAConfigMap`.
func (c *HPAWatcherClient) ReadConfigMap() []CustomExternalMetric {
	var list []CustomExternalMetric
	cm, err := c.clientSet.CoreV1().ConfigMaps(c.ns).Get(datadogHPAConfigMap, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Could not store the custom metrics data in the ConfigMap %s: %s", datadogHPAConfigMap, err.Error())
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

// Stop sends a signal to the HPAWatcher to stop it.
// Used for the tests to avoid leaking go-routines.
func (c *HPAWatcherClient) Stop() {
	c.pollItl.Stop()
	c.refreshItl.Stop()
}

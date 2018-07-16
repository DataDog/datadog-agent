// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"reflect"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/zorkian/go-datadog-api.v2"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	expectedHPAType = reflect.TypeOf(autoscalingv2.HorizontalPodAutoscaler{})
)

type DatadogClient interface {
	QueryMetrics(from, to int64, query string) ([]datadog.Series, error)
}

// HPAWatcherClient embeds the API Server client and the configuration to refresh metrics from Datadog and watch the HPA Objects' activities
type HPAWatcherClient struct {
	clientSet      kubernetes.Interface
	readTimeout    time.Duration
	refreshItl     *time.Ticker
	pollItl        *time.Ticker
	externalMaxAge time.Duration
	datadogClient  DatadogClient
	store          custommetrics.Store
}

// NewHPAWatcherClient returns a new HPAWatcherClient
func NewHPAWatcherClient(clientSet kubernetes.Interface, datadogCl DatadogClient, store custommetrics.Store) (*HPAWatcherClient, error) {
	pollInterval := config.Datadog.GetInt("hpa_watcher_polling_freq")
	refreshInterval := config.Datadog.GetInt("external_metrics_provider.polling_freq")
	externalMaxAge := config.Datadog.GetInt("external_metrics_provider.max_age")
	return &HPAWatcherClient{
		clientSet:      clientSet,
		readTimeout:    100 * time.Millisecond,
		pollItl:        time.NewTicker(time.Duration(pollInterval) * time.Second),
		refreshItl:     time.NewTicker(time.Duration(refreshInterval) * time.Second),
		externalMaxAge: time.Duration(externalMaxAge) * time.Second,
		datadogClient:  datadogCl,
		store:          store,
	}, nil
}

func (c *HPAWatcherClient) run(res string) (added, modified, deleted []*autoscalingv2.HorizontalPodAutoscaler, resVer string, err error) {
	metaOptions := metav1.ListOptions{Watch: true, ResourceVersion: res}
	watcher, err := c.clientSet.AutoscalingV2beta1().HorizontalPodAutoscalers(metav1.NamespaceAll).Watch(metaOptions)
	if err != nil {
		log.Infof("Failed to watch %v: %v", expectedHPAType, err)
	}
	defer watcher.Stop()

	watcherTimeout := time.NewTimer(c.readTimeout)
	for {
		select {
		case rcvdHPA, ok := <-watcher.ResultChan():
			if !ok {
				log.Debugf("Unexpected watch close")
				return nil, nil, nil, "0", err
			}
			currHPA, ok := rcvdHPA.Object.(*autoscalingv2.HorizontalPodAutoscaler)
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
				added = append(added, currHPA)
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
			return added, modified, deleted, resVer, nil
		}
	}
}

// Start runs a watch process of the various HPA objects' activities to process and store the relevant info.
// Refreshes the custom metrics stored as well.
func (c *HPAWatcherClient) Start() {
	log.Info("Starting HPA Process ...")
	tickerHPAWatchProcess := c.pollItl
	tickerHPARefreshProcess := c.refreshItl

	// Creating a leader election engine to make sure only the leader writes the metrics in the configmap and queries Datadog.
	leaderEngine, err := leaderelection.GetLeaderEngine()
	if err != nil {
		log.Errorf("Could not ensure the leader election is running properly: %s", err)
		return
	}
	leaderEngine.EnsureLeaderElectionRuns()

	var resversion string

	go func() {
		for {
			select {
			// Ticker for the HPA Object watcher
			case <-tickerHPAWatchProcess.C:
				if !leaderEngine.IsLeader() {
					continue
				}
				added, modified, deleted, res, err := c.run(resversion)
				if err != nil {
					log.Errorf("Error while watching HPA Objects' activities: %s", err)
					return
				}
				if res != resversion && res != "" {
					resversion = res

					var err error
					if err = c.processHPAs(added, modified); err != nil {
						log.Errorf("Could not update the external metrics in the store: %s", err.Error())
					}
					if err = c.removeEntryFromStore(deleted); err != nil {
						log.Errorf("Could not delete the external metrics in the store: %s", err.Error())
					}
				}
			// Ticker to run the refresh process for the stored external metrics
			case <-tickerHPARefreshProcess.C:
				if !leaderEngine.IsLeader() {
					continue
				}
				// Updating the metrics against Datadog should not affect the HPA pipeline.
				// If metrics are temporarily unavailable for too long, they will become `Valid=false` and won't be evaluated.
				c.updateExternalMetrics()
			}
		}
	}()
}

func (c *HPAWatcherClient) updateExternalMetrics() {
	maxAge := int64(c.externalMaxAge.Seconds())

	emList, err := c.store.ListAllExternalMetricValues()
	if err != nil {
		log.Infof("Error while retrieving external metrics from the store: %s", err)
		return
	}

	if len(emList) == 0 {
		log.Debugf("No External Metrics to evaluate at the moment")
		return
	}

	var updated []custommetrics.ExternalMetricValue
	for _, em := range emList {
		if metav1.Now().Unix()-em.Timestamp <= maxAge && em.Valid {
			continue
		}
		em.Valid = false
		em.Timestamp = metav1.Now().Unix()
		em.Value, em.Valid, err = c.validateExternalMetric(em.MetricName, em.Labels)
		if err != nil {
			log.Debugf("Could not fetch the external metric %s from Datadog, metric is no longer valid: %s", em.MetricName, err)
		}
		log.Debugf("Updated the external metric %#v", em)
		updated = append(updated, em)
	}
	if err = c.store.SetExternalMetricValues(updated); err != nil {
		log.Errorf("Could not update the external metrics in the store: %s", err.Error())
	}
}

// processHPAs transforms HPA data into structures to be stored upon validation that the metrics are available in Datadog
func (c *HPAWatcherClient) processHPAs(added, modified []*autoscalingv2.HorizontalPodAutoscaler) error {
	added = append(added, modified...)
	if len(added) == 0 {
		return nil
	}
	var externalMetrics []custommetrics.ExternalMetricValue
	var err error
	for _, hpa := range added {
		for _, metricSpec := range hpa.Spec.Metrics {
			switch metricSpec.Type {
			case autoscalingv2.ExternalMetricSourceType:
				m := custommetrics.ExternalMetricValue{
					MetricName: metricSpec.External.MetricName,
					Timestamp:  metav1.Now().Unix(),
					HPA: custommetrics.ObjectReference{
						Name:      hpa.Name,
						Namespace: hpa.Namespace,
					},
					Labels: metricSpec.External.MetricSelector.MatchLabels,
				}
				m.Value, m.Valid, err = c.validateExternalMetric(m.MetricName, m.Labels)
				if err != nil {
					log.Debugf("Could not fetch the external metric %s from Datadog, metric is no longer valid: %s", m.MetricName, err)
				}
				externalMetrics = append(externalMetrics, m)
			default:
				log.Debugf("Unsupported metric type %s", metricSpec.Type)
			}
		}
	}
	return c.store.SetExternalMetricValues(externalMetrics)
}

func (c *HPAWatcherClient) removeEntryFromStore(deleted []*autoscalingv2.HorizontalPodAutoscaler) error {
	if len(deleted) == 0 {
		return nil
	}
	objectRefs := []custommetrics.ObjectReference{}
	for _, hpa := range deleted {
		objectRef := custommetrics.ObjectReference{
			Name:      hpa.Name,
			Namespace: hpa.Namespace,
		}
		objectRefs = append(objectRefs, objectRef)
	}
	return c.store.Delete(objectRefs)
}

// validateExternalMetric queries Datadog to validate the availability of an external metric
func (c *HPAWatcherClient) validateExternalMetric(metricName string, labels map[string]string) (value int64, valid bool, err error) {
	val, err := c.queryDatadogExternal(metricName, labels)
	if err != nil {
		return val, false, err
	}
	return val, true, nil
}

// Stop sends a signal to the HPAWatcher to stop it.
// Used for the tests to avoid leaking go-routines.
func (c *HPAWatcherClient) Stop() {
	c.pollItl.Stop()
	c.refreshItl.Stop()
}

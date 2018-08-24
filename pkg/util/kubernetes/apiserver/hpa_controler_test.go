// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/zorkian/go-datadog-api.v2"
	"k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hpa"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
)

func newFakeConfigMapStore(t *testing.T, ns, name string, metrics []custommetrics.ExternalMetricValue) (custommetrics.Store, kubernetes.Interface) {
	client := fake.NewSimpleClientset()
	store, err := custommetrics.NewConfigMapStore(client, ns, name)
	require.NoError(t, err)
	err = store.SetExternalMetricValues(metrics)
	require.NoError(t, err)
	return store, client
}

func newFakeHorizontalPodAutoscaler(name, ns string, uid string, metricName string, labels map[string]string) *v2beta1.HorizontalPodAutoscaler {
	return &v2beta1.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			UID:       types.UID(uid),
		},
		Spec: v2beta1.HorizontalPodAutoscalerSpec{
			Metrics: []v2beta1.MetricSpec{
				{
					Type: v2beta1.ExternalMetricSourceType,
					External: &v2beta1.ExternalMetricSource{
						MetricName: metricName,
						MetricSelector: &metav1.LabelSelector{
							MatchLabels: labels,
						},
					},
				},
			},
		},
	}
}

func newFakeAutoscalerController(client kubernetes.Interface, itf LeaderElectorItf, dcl hpa.DatadogClient) (*AutoscalersController, informers.SharedInformerFactory) {
	informerFactory := informers.NewSharedInformerFactory(client, 0)

	autoscalerController := NewAutoscalerController(
		client,
		itf,
		dcl,
		informerFactory.Autoscaling().V2beta1().HorizontalPodAutoscalers(),
	)
	autoscalerController.autoscalersListerSynced = alwaysReady

	return autoscalerController, informerFactory
}

type leItf struct{}

func (le *leItf) IsLeader() bool { return true }

type dClItf struct {
	queryMetricsFunc func(from, to int64, query string) ([]datadog.Series, error)
}

func (d *dClItf) QueryMetrics(from, to int64, query string) ([]datadog.Series, error) {
	if d.queryMetricsFunc != nil {
		return d.queryMetricsFunc(from, to, query)
	}
	return nil, nil
}

// TestAutoscalerController is an integration test of the AutoscalerController
func TestAutoscalerController(t *testing.T) {
	client := fake.NewSimpleClientset()
	i := &leItf{}
	metricName := "foo"
	name := custommetrics.GetConfigmapName()
	ddSeries := []datadog.Series{
		{
			Metric: &metricName,
			Points: []datadog.DataPoint{
				{1531492452, 12},
				{1531492486, 14},
			},
		},
	}
	d := &dClItf{
		queryMetricsFunc: func(from, to int64, query string) ([]datadog.Series, error) {
			return ddSeries, nil
		},
	}
	hctrl, inf := newFakeAutoscalerController(client, LeaderElectorItf(i), hpa.DatadogClient(d))
	hctrl.poller.batchWindow = 60 // Do not trigger the refresh or batch call to avoid flakiness.
	hctrl.poller.refreshPeriod = 60
	hctrl.autoscalers = make(chan interface{}, 1)

	stop := make(chan struct{})
	defer close(stop)
	inf.Start(stop)
	go hctrl.Run(stop)

	c := client.AutoscalingV2beta1()
	require.NotNil(t, c)

	mockedHPA := newFakeHorizontalPodAutoscaler(
		"hpa_1",
		"default",
		"1",
		"foo",
		map[string]string{"foo": "bar"},
	)

	_, err := c.HorizontalPodAutoscalers("default").Create(mockedHPA)
	require.NoError(t, err)
	timeout := time.NewTimer(5 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	select {
	case <-hctrl.autoscalers:
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}
	// Check local cache store is 1:1 with expectations
	storedHPA, err := hctrl.autoscalersLister.HorizontalPodAutoscalers(mockedHPA.Namespace).Get(mockedHPA.Name)
	require.Equal(t, storedHPA, mockedHPA)

	select {
	case <-ticker.C:
		st := hctrl.hpaToStoreGlobally
		require.NotEmpty(t, st)
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}
	// Forcing update the to global store and clean local cache store
	hctrl.pushToGlobalStore(hctrl.hpaToStoreGlobally)
	require.Nil(t, hctrl.hpaToStoreGlobally)

	// Test that the Global store contains the correct data
	select {
	case <-ticker.C:
		k, _ := client.CoreV1().ConfigMaps("default").Get(name, metav1.GetOptions{})
		for _, v := range k.Data {
			require.Contains(t, v, "\"value\":14")
			require.Contains(t, v, "\"labels\":{\"foo\":\"bar\"}")
		}
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}

	// Update the Metrics
	mockedHPA.Spec.Metrics = []v2beta1.MetricSpec{
		{
			Type: v2beta1.ExternalMetricSourceType,
			External: &v2beta1.ExternalMetricSource{
				MetricName: "foo",
				MetricSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"dcos_version": "2.1.9",
					},
				},
			},
		},
	}
	_, err = c.HorizontalPodAutoscalers(mockedHPA.Namespace).Update(mockedHPA)
	require.NoError(t, err)
	select {
	case <-hctrl.autoscalers:
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}

	storedHPA, err = hctrl.autoscalersLister.HorizontalPodAutoscalers(mockedHPA.Namespace).Get(mockedHPA.Name)
	require.NoError(t, err)
	require.Equal(t, storedHPA, mockedHPA)

	// Process and submit to the Global Store
	// bypassing hpaToStoreGlobally, although it could be used
	toStore := hctrl.hpaProc.ProcessHPAs(storedHPA)
	errPush := hctrl.pushToGlobalStore(toStore)
	require.NoError(t, errPush)
	select {
	case <-ticker.C:
		k, _ := client.CoreV1().ConfigMaps("default").Get(name, metav1.GetOptions{})
		for _, v := range k.Data {
			require.Contains(t, v, "\"labels\":{\"dcos_version\":\"2.1.9\"}")
			require.Contains(t, v, "\"valid\":true")
		}
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}

	// Verify that a Delete removes the Data from the Global Store
	err = c.HorizontalPodAutoscalers("default").Delete(mockedHPA.Name, &metav1.DeleteOptions{})
	require.NoError(t, err)
	select {
	case <-ticker.C:
		k, err := client.CoreV1().ConfigMaps("default").Get(name, metav1.GetOptions{})
		require.NoError(t, err)
		require.Len(t, k.Data, 0)
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}
}

// TestAutoscalerControllerGC tests the GC process of of the controller
func TestAutoscalerControllerGC(t *testing.T) {
	testCases := []struct {
		caseName string
		metrics  []custommetrics.ExternalMetricValue
		hpa      *v2beta1.HorizontalPodAutoscaler
		expected []custommetrics.ExternalMetricValue
	}{
		{
			caseName: "hpa exists for metric",
			metrics: []custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"bar": "baz"},
					HPA:        custommetrics.ObjectReference{Name: "foo", Namespace: "default", UID: "1111"},
					Timestamp:  12,
					Value:      1,
					Valid:      false,
				},
			},
			hpa: &v2beta1.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					UID:       "1111",
				},
			},
			expected: []custommetrics.ExternalMetricValue{ // skipped by gc
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"bar": "baz"},
					HPA:        custommetrics.ObjectReference{Name: "foo", Namespace: "default", UID: "1111"},
					Timestamp:  12,
					Value:      1,
					Valid:      false,
				},
			},
		},
		{
			caseName: "no hpa for metric",
			metrics: []custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"bar": "baz"},
					HPA:        custommetrics.ObjectReference{Name: "foo", Namespace: "default", UID: "1111"},
					Timestamp:  12,
					Value:      1,
					Valid:      false,
				},
			},
			expected: []custommetrics.ExternalMetricValue{},
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("#%d %s", i, testCase.caseName), func(t *testing.T) {
			store, client := newFakeConfigMapStore(t, "default", fmt.Sprintf("test-%d", i), testCase.metrics)

			h := &AutoscalersController{
				store:     store,
				clientSet: client,
			}
			if testCase.hpa != nil {
				_, err := client.
					AutoscalingV2beta1().
					HorizontalPodAutoscalers("default").
					Create(testCase.hpa)
				require.NoError(t, err)
			}

			h.gc() // force gc to run
			allMetrics, err := store.ListAllExternalMetricValues()
			require.NoError(t, err)
			assert.ElementsMatch(t, testCase.expected, allMetrics)
		})
	}
}

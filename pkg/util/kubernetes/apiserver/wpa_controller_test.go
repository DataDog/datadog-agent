// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	wpa_client "github.com/DataDog/watermarkpodautoscaler/pkg/client/clientset/versioned"
	"github.com/stretchr/testify/require"
	"gopkg.in/zorkian/go-datadog-api.v2"
	"k8s.io/client-go/kubernetes"

	wpa_informers "github.com/DataDog/watermarkpodautoscaler/pkg/client/informers/externalversions"
	"k8s.io/apimachinery/pkg/types"
	k8s_fake "k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/watermarkpodautoscaler/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/watermarkpodautoscaler/pkg/client/clientset/versioned/fake"
	"k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

// TestupdateExternalMetrics checks the reconciliation between the local cache and the global store logic
func TestUpdateWPA(t *testing.T) {
	name := custommetrics.GetConfigmapName()
	store, client := newFakeConfigMapStore(t, "default", name, nil)
	d := &fakeDatadogClient{}

	p := &fakeProcessor{
		updateMetricFunc: func(emList map[string]custommetrics.ExternalMetricValue) (updated map[string]custommetrics.ExternalMetricValue) {
			updated = make(map[string]custommetrics.ExternalMetricValue)
			for id, m := range emList {
				m.Valid = true
				updated[id] = m
			}
			return updated
		},
	}

	hctrl, _ := newFakeAutoscalerController(client, alwaysLeader, autoscalers.DatadogClient(d))
	hctrl.poller.refreshPeriod = 600
	hctrl.poller.gcPeriodSeconds = 600
	hctrl.autoscalers = make(chan interface{}, 1)
	foo := autoscalers.ProcessorInterface(p)
	hctrl.hpaProc = foo

	// Fresh start with no activity. Both the local cache and the Global Store are empty.
	hctrl.updateExternalMetrics()
	metrics, err := store.ListAllExternalMetricValues()
	require.NoError(t, err)
	require.Len(t, metrics, 0)

	// Start the DCA with already existing Data
	// Check if nothing in local store and Global Store is full we update the Global Store metrics correctly
	metricsToStore := map[string]custommetrics.ExternalMetricValue{
		"external_metric-watermark-default-foo-metric1": {
			MetricName: "metric1",
			Labels:     map[string]string{"foo": "bar"},
			Ref: custommetrics.ObjectReference{
				Type:      "watermark",
				Name:      "foo",
				Namespace: "default",
			},
			Value: 1.3,
			Valid: true,
		},
	}
	store.SetExternalMetricValues(metricsToStore)
	// Check that the store is up to date
	metrics, err = store.ListAllExternalMetricValues()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	hctrl.toStore.m.Lock()
	require.Len(t, hctrl.toStore.data, 0)
	hctrl.toStore.m.Unlock()

	hctrl.updateExternalMetrics()

	metrics, err = store.ListAllExternalMetricValues()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	hctrl.toStore.m.Lock()
	require.Len(t, hctrl.toStore.data, 0)
	hctrl.toStore.m.Unlock()

	// Fresh start
	// Check if local store is not empty
	hctrl.toStore.m.Lock()
	hctrl.toStore.data["external_metric-watermark-default-foo-metric2"] = custommetrics.ExternalMetricValue{
		MetricName: "metric2",
		Labels:     map[string]string{"foo": "bar"},
		Ref: custommetrics.ObjectReference{
			Type:      "watermark",
			Name:      "foo",
			Namespace: "default",
		},
	}
	require.Len(t, hctrl.toStore.data, 1)
	hctrl.toStore.m.Unlock()

	hctrl.updateExternalMetrics()
	metrics, err = store.ListAllExternalMetricValues()
	require.NoError(t, err)
	require.Len(t, metrics, 2)

	// DCA becomes leader
	// Check that if there is conflicting info from the local store and the Global Store that we merge correctly
	// Check conflict on metric name and labels
	hctrl.toStore.m.Lock()
	hctrl.toStore.data["external_metric-watermark-default-foo-metric2"] = custommetrics.ExternalMetricValue{
		MetricName: "metric2",
		Labels:     map[string]string{"foo": "baz"},
		Ref: custommetrics.ObjectReference{
			Type:      "watermark",
			Name:      "foo",
			Namespace: "default",
		},
	}
	require.Len(t, hctrl.toStore.data, 1)
	hctrl.toStore.m.Unlock()
	hctrl.updateExternalMetrics()
	metrics, err = store.ListAllExternalMetricValues()
	require.NoError(t, err)
	require.Len(t, metrics, 2)

	for _, m := range metrics {
		require.True(t, m.Valid)
		if m.MetricName == "metric2" {
			require.True(t, reflect.DeepEqual(m.Labels, map[string]string{"foo": "baz"}))
		}
	}

}

func newFakeWPAController(kubeClient kubernetes.Interface, client wpa_client.Interface, itf LeaderElectorInterface, dcl autoscalers.DatadogClient) (*AutoscalersController, wpa_informers.SharedInformerFactory) {
	// need to fake wpa_client.
	inf := wpa_informers.NewSharedInformerFactory(client, 0)
	autoscalerController, _ := NewAutoscalersController(
		kubeClient,
		itf,
		dcl,
	)

	ExtendToWPAController(autoscalerController, inf.Datadoghq().V1alpha1().WatermarkPodAutoscalers())

	autoscalerController.autoscalersListerSynced = func() bool { return true }

	return autoscalerController, inf
}

func newFakeWatermarkPodAutoscaler(name, ns string, uid string, metricName string, labels map[string]string) *v1alpha1.WatermarkPodAutoscaler {
	return &v1alpha1.WatermarkPodAutoscaler{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			UID:       types.UID(uid),
		},
		Spec: v1alpha1.WatermarkPodAutoscalerSpec{
			Metrics: []v1alpha1.MetricSpec{
				{
					Type: v1alpha1.ExternalMetricSourceType,
					External: &v1alpha1.ExternalMetricSource{
						MetricName: metricName,
						MetricSelector: &v1.LabelSelector{
							MatchLabels: labels,
						},
					},
				},
			},
		},
	}
}

// TestAutoscalerController is an integration test of the AutoscalerController
func TestWPAController(t *testing.T) {
	penTime := (int(time.Now().Unix()) - int(maxAge.Seconds()/2)) * 1000
	name := custommetrics.GetConfigmapName()
	store, client := newFakeConfigMapStore(t, "default", name, nil)
	metricName := "foo"
	ddSeries := []datadog.Series{
		{
			Metric: &metricName,
			Points: []datadog.DataPoint{
				makePoints(1531492452000, 12.01),
				makePoints(penTime, 14.123),
				makePoints(0, 25.12),
			},
			Scope: makePtr("foo:bar"),
		},
		{
			Metric: &metricName,
			Points: []datadog.DataPoint{
				makePoints(1531492452000, 12.34),
				makePoints(penTime, 1.01),
				makePoints(0, 0.902),
			},
			Scope: makePtr("dcos_version:2.1.9"),
		},
	}
	d := &fakeDatadogClient{
		queryMetricsFunc: func(from, to int64, query string) ([]datadog.Series, error) {
			return ddSeries, nil
		},
	}
	wpaClient := fake.NewSimpleClientset()
	hctrl, inf := newFakeWPAController(client, wpaClient, alwaysLeader, autoscalers.DatadogClient(d))
	hctrl.poller.refreshPeriod = 600
	hctrl.poller.gcPeriodSeconds = 600
	hctrl.autoscalers = make(chan interface{}, 1)

	stop := make(chan struct{})
	defer close(stop)
	inf.Start(stop)

	go hctrl.RunWPA(stop)

	hctrl.RunControllerLoop(stop)

	c := wpaClient.DatadoghqV1alpha1()
	require.NotNil(t, c)

	mockedWPA := newFakeWatermarkPodAutoscaler(
		"hpa_1",
		"default",
		"1",
		"foo",
		map[string]string{"foo": "bar"},
	)
	mockedWPA.Annotations = makeAnnotations("foo", map[string]string{"foo": "bar"})

	_, err := c.WatermarkPodAutoscalers("default").Create(mockedWPA)
	require.NoError(t, err)
	timeout := time.NewTimer(5 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	select {
	case <-hctrl.autoscalers:
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}
	// Check local cache store is 1:1 with expectations
	storedWPA, err := hctrl.wpaLister.WatermarkPodAutoscalers(mockedWPA.Namespace).Get(mockedWPA.Name)
	require.NoError(t, err)
	require.Equal(t, storedWPA, mockedWPA)
	select {
	case <-ticker.C:
		hctrl.toStore.m.Lock()
		st := hctrl.toStore.data
		hctrl.toStore.m.Unlock()
		require.NotEmpty(t, st)
		require.Len(t, st, 1)
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}

	hctrl.updateExternalMetrics()

	// Test that the Global store contains the correct data
	select {
	case <-ticker.C:
		storedExternal, err := store.ListAllExternalMetricValues()
		require.NoError(t, err)
		require.NotZero(t, len(storedExternal))
		require.Equal(t, storedExternal[0].Value, float64(14.123))
		require.Equal(t, storedExternal[0].Labels, map[string]string{"foo": "bar"})
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}

	// Update the Metrics
	mockedWPA.Spec.Metrics = []v1alpha1.MetricSpec{
		{
			Type: v1alpha1.ExternalMetricSourceType,
			External: &v1alpha1.ExternalMetricSource{
				MetricName: "foo",
				MetricSelector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						"dcos_version": "2.1.9",
					},
				},
			},
		},
	}
	mockedWPA.Annotations = makeAnnotations("nginx.net.request_per_s", map[string]string{"dcos_version": "2.1.9"})
	_, err = c.WatermarkPodAutoscalers(mockedWPA.Namespace).Update(mockedWPA)
	require.NoError(t, err)
	select {
	case <-hctrl.autoscalers:
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}

	storedWPA, err = hctrl.wpaLister.WatermarkPodAutoscalers(mockedWPA.Namespace).Get(mockedWPA.Name)
	require.NoError(t, err)
	require.Equal(t, storedWPA, mockedWPA)
	// Checking the local cache holds the correct Data.
	ExtVal := autoscalers.InspectWPA(storedWPA)
	key := custommetrics.ExternalMetricValueKeyFunc(ExtVal[0])

	// Process and submit to the Global Store
	select {
	case <-ticker.C:
		hctrl.toStore.m.Lock()
		st := hctrl.toStore.data
		hctrl.toStore.m.Unlock()
		require.NotEmpty(t, st)
		require.Len(t, st, 1)
		// Not comparing timestamps to avoid flakyness.
		require.Equal(t, ExtVal[0].Ref, st[key].Ref)
		require.Equal(t, ExtVal[0].MetricName, st[key].MetricName)
		require.Equal(t, ExtVal[0].Labels, st[key].Labels)
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}

	hctrl.updateExternalMetrics()

	select {
	case <-ticker.C:
		storedExternal, err := store.ListAllExternalMetricValues()
		require.NoError(t, err)
		require.NotZero(t, len(storedExternal))
		require.Equal(t, storedExternal[0].Value, float64(1.01))
		require.Equal(t, storedExternal[0].Labels, map[string]string{"dcos_version": "2.1.9"})
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for WPAs to update")
	}

	// Verify that a Delete removes the Data from the Global Store
	err = c.WatermarkPodAutoscalers("default").Delete(mockedWPA.Name, &v1.DeleteOptions{})
	require.NoError(t, err)
	select {
	case <-ticker.C:
		storedExternal, err := store.ListAllExternalMetricValues()
		require.NoError(t, err)
		require.Len(t, storedExternal, 0)
		hctrl.toStore.m.Lock()
		st := hctrl.toStore.data
		hctrl.toStore.m.Unlock()
		require.Len(t, st, 0)

	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}
}

func TestWPASync(t *testing.T) {
	client := k8s_fake.NewSimpleClientset()
	wpaClient := fake.NewSimpleClientset()
	d := &fakeDatadogClient{}
	hctrl, inf := newFakeWPAController(client, wpaClient, alwaysLeader, d)

	obj := newFakeWatermarkPodAutoscaler(
		"wpa_1",
		"default",
		"1",
		"foo",
		map[string]string{"foo": "bar"},
	)

	err := inf.Datadoghq().V1alpha1().WatermarkPodAutoscalers().Informer().GetStore().Add(obj)
	require.NoError(t, err)
	key := "default/wpa_1"
	err = hctrl.syncWatermarkPoAutoscalers(key)
	require.NoError(t, err)

	fakeKey := "default/prometheus"
	err = hctrl.syncWatermarkPoAutoscalers(fakeKey)
	require.Error(t, err, errors.IsNotFound)

}

// TestAutoscalerControllerGC tests the GC process of of the controller
func TestWPAGC(t *testing.T) {
	testCases := []struct {
		caseName string
		metrics  map[string]custommetrics.ExternalMetricValue
		wpa      *v1alpha1.WatermarkPodAutoscaler
		expected []custommetrics.ExternalMetricValue
	}{
		{
			caseName: "wpa exists for metric",
			metrics: map[string]custommetrics.ExternalMetricValue{
				"external_metric-watermark-default-foo-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"bar": "baz"},
					Ref:        custommetrics.ObjectReference{Type: "watermark", Name: "foo", Namespace: "default", UID: "1111"},
					Timestamp:  12,
					Value:      1,
					Valid:      false,
				},
			},
			wpa: &v1alpha1.WatermarkPodAutoscaler{
				ObjectMeta: v1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					UID:       "1111",
				},
				Spec: v1alpha1.WatermarkPodAutoscalerSpec{
					Metrics: []v1alpha1.MetricSpec{
						{
							Type: v1alpha1.ExternalMetricSourceType,
							External: &v1alpha1.ExternalMetricSource{
								MetricName: "requests_per_s",
								MetricSelector: &v1.LabelSelector{
									MatchLabels: map[string]string{"bar": "baz"},
								},
							},
						},
					},
				},
			},
			expected: []custommetrics.ExternalMetricValue{ // skipped by gc
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"bar": "baz"},
					Ref:        custommetrics.ObjectReference{Type: "watermark", Name: "foo", Namespace: "default", UID: "1111"},
					Timestamp:  12,
					Value:      1,
					Valid:      false,
				},
			},
		},
		{
			caseName: "no wpa for metric",
			metrics: map[string]custommetrics.ExternalMetricValue{
				"external_metric-watermark-default-foo-requests_per_s_b": {
					MetricName: "requests_per_s_b",
					Labels:     map[string]string{"bar": "baz"},
					Ref:        custommetrics.ObjectReference{Type: "watermark", Name: "foo", Namespace: "default", UID: "1111"},
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
			i := &fakeLeaderElector{}
			d := &fakeDatadogClient{}
			wpaCl := fake.NewSimpleClientset()
			hctrl, inf := newFakeWPAController(client, wpaCl, i, d)

			hctrl.store = store

			if testCase.wpa != nil {
				err := inf.Datadoghq().
					V1alpha1().
					WatermarkPodAutoscalers().
					Informer().
					GetStore().
					Add(testCase.wpa)
				require.NoError(t, err)
			}
			hctrl.gc() // force gc to run
			allMetrics, err := store.ListAllExternalMetricValues()
			require.NoError(t, err)
			assert.ElementsMatch(t, testCase.expected, allMetrics)
		})
	}
}

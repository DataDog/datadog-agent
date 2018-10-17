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

	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hpa"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
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

func newFakeAutoscalerController(client kubernetes.Interface, itf LeaderElectorInterface, dcl hpa.DatadogClient) (*AutoscalersController, informers.SharedInformerFactory) {
	informerFactory := informers.NewSharedInformerFactory(client, 0)

	autoscalerController, _ := NewAutoscalersController(
		client,
		itf,
		dcl,
		informerFactory.Autoscaling().V2beta1().HorizontalPodAutoscalers(),
	)

	autoscalerController.autoscalersListerSynced = func() bool { return true }

	return autoscalerController, informerFactory
}

var alwaysLeader = &fakeLeaderElector{true}

type fakeLeaderElector struct {
	isLeader bool
}

func (le *fakeLeaderElector) IsLeader() bool { return le.isLeader }

type fakeDatadogClient struct {
	queryMetricsFunc func(from, to int64, query string) ([]datadog.Series, error)
}

func (d *fakeDatadogClient) QueryMetrics(from, to int64, query string) ([]datadog.Series, error) {
	if d.queryMetricsFunc != nil {
		return d.queryMetricsFunc(from, to, query)
	}
	return nil, nil
}

var maxAge = time.Duration(30 * time.Second)

func makePoints(ts, val int) datadog.DataPoint {
	if ts == 0 {
		ts = (int(metav1.Now().Unix()) - int(maxAge.Seconds()/2)) * 1000 // use ms
	}
	tsPtr := float64(ts)
	valPtr := float64(val)
	return datadog.DataPoint{&tsPtr, &valPtr}
}

func makePtr(val string) *string {
	return &val
}

// TestAutoscalerController is an integration test of the AutoscalerController
func TestAutoscalerController(t *testing.T) {
	name := custommetrics.GetConfigmapName()
	store, client := newFakeConfigMapStore(t, "default", name, nil)
	metricName := "foo"
	ddSeries := []datadog.Series{
		{
			Metric: &metricName,
			Points: []datadog.DataPoint{
				makePoints(1531492452000, 12),
				makePoints(0, 14),
			},
			Scope: makePtr("foo:bar"),
		},
		{
			Metric: &metricName,
			Points: []datadog.DataPoint{
				makePoints(1531492452000, 12),
				makePoints(0, 11),
			},
			Scope: makePtr("dcos_version:2.1.9"),
		},
	}
	d := &fakeDatadogClient{
		queryMetricsFunc: func(from, to int64, query string) ([]datadog.Series, error) {
			return ddSeries, nil
		},
	}
	hctrl, inf := newFakeAutoscalerController(client, alwaysLeader, hpa.DatadogClient(d))
	hctrl.poller.batchWindow = 600 // Do not trigger the refresh or batch call to avoid flakiness.
	hctrl.poller.refreshPeriod = 600
	hctrl.poller.gcPeriodSeconds = 600
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
	require.NoError(t, err)
	require.Equal(t, storedHPA, mockedHPA)
	select {
	case <-ticker.C:
		hctrl.toStore.m.Lock()
		st := hctrl.toStore.data
		hctrl.toStore.m.Unlock()
		require.NotEmpty(t, st)
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}
	// Forcing update the to global store and clean local cache store
	hctrl.pushToGlobalStore()
	require.Nil(t, hctrl.toStore.data)

	// Test that the Global store contains the correct data
	select {
	case <-ticker.C:
		storedExternal, err := store.ListAllExternalMetricValues()
		require.NoError(t, err)
		require.NotZero(t, len(storedExternal))
		require.Equal(t, storedExternal[0].Value, int64(14))
		require.Equal(t, storedExternal[0].Labels, map[string]string{"foo": "bar"})
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
	select {
	case <-ticker.C:
		hctrl.toStore.m.Lock()
		st := hctrl.toStore.data // ensure toStore is not nil before pushToGlobalStore
		hctrl.toStore.m.Unlock()
		require.NotEmpty(t, st)
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}

	errPush := hctrl.pushToGlobalStore()

	require.NoError(t, errPush)
	select {
	case <-ticker.C:
		storedExternal, err := store.ListAllExternalMetricValues()
		require.NoError(t, err)
		require.NotZero(t, len(storedExternal))
		require.Equal(t, storedExternal[0].Value, int64(11))
		require.Equal(t, storedExternal[0].Labels, map[string]string{"dcos_version": "2.1.9"})
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}

	// Verify that a Delete removes the Data from the Global Store
	err = c.HorizontalPodAutoscalers("default").Delete(mockedHPA.Name, &metav1.DeleteOptions{})
	require.NoError(t, err)
	select {
	case <-ticker.C:
		storedExternal, err := store.ListAllExternalMetricValues()
		require.NoError(t, err)
		require.Len(t, storedExternal, 0)
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}
}

func TestAutoscalerSync(t *testing.T) {
	client := fake.NewSimpleClientset()
	d := &fakeDatadogClient{}
	hctrl, inf := newFakeAutoscalerController(client, alwaysLeader, d)
	obj := newFakeHorizontalPodAutoscaler(
		"hpa_1",
		"default",
		"1",
		"foo",
		map[string]string{"foo": "bar"},
	)

	err := inf.Autoscaling().V2beta1().HorizontalPodAutoscalers().Informer().GetStore().Add(obj)
	require.NoError(t, err)
	key := "default/hpa_1"
	err = hctrl.syncAutoscalers(key)
	require.NoError(t, err)

	fakeKey := "default/prometheus"
	err = hctrl.syncAutoscalers(fakeKey)
	require.Error(t, err, errors.IsNotFound)

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
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								MetricName: "requests_per_s",
								MetricSelector: &metav1.LabelSelector{
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
			i := &fakeLeaderElector{}
			d := &fakeDatadogClient{}
			hctrl, inf := newFakeAutoscalerController(client, i, d)

			hctrl.store = store

			if testCase.hpa != nil {
				err := inf.
					Autoscaling().
					V2beta1().
					HorizontalPodAutoscalers().
					Informer().
					GetStore().
					Add(testCase.hpa)
				require.NoError(t, err)
			}
			hctrl.gc() // force gc to run
			allMetrics, err := store.ListAllExternalMetricValues()
			require.NoError(t, err)
			assert.ElementsMatch(t, testCase.expected, allMetrics)
		})
	}
}

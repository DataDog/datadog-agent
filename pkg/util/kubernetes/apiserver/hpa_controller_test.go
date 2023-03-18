// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/zorkian/go-datadog-api.v2"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	autoscalingGroup = "autoscaling"
	hpaResource      = "horizontalpodautoscalers"
)

func newClient() kubernetes.Interface {
	client := fake.NewSimpleClientset()
	client.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: fmt.Sprintf("%s/%s", autoscalingGroup, "v2beta1"),
			APIResources: []metav1.APIResource{
				{
					Name:    hpaResource,
					Group:   autoscalingGroup,
					Version: "v2beta1",
				},
			},
		},
	}
	return client
}

func newFakeConfigMapStore(t *testing.T, ns, name string, metrics map[string]custommetrics.ExternalMetricValue) (custommetrics.Store, kubernetes.Interface) {
	client := newClient()
	store, err := custommetrics.NewConfigMapStore(client, ns, name)
	require.NoError(t, err)
	err = store.SetExternalMetricValues(metrics)
	require.NoError(t, err)
	return store, client
}

func newFakeHorizontalPodAutoscaler(name, ns string, uid string, metricName string, labels map[string]string) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			UID:       types.UID(uid),
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ExternalMetricSourceType,
					External: &autoscalingv2.ExternalMetricSource{
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

func newFakeAutoscalerController(t *testing.T, client kubernetes.Interface, isLeaderFunc func() bool, dcl autoscalers.DatadogClient) (*AutoscalersController, informers.SharedInformerFactory) {
	informerFactory := informers.NewSharedInformerFactory(client, 0)

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(t.Logf)

	autoscalerController, _ := NewAutoscalersController(
		client,
		eventBroadcaster.NewRecorder(kscheme.Scheme, corev1.EventSource{Component: "FakeAutoscalerController"}),
		isLeaderFunc,
		dcl,
	)
	autoscalerController.enableHPA(client, informerFactory)

	autoscalerController.autoscalersListerSynced = func() bool { return true }

	return autoscalerController, informerFactory
}

var alwaysLeader = func() bool { return true }

type fakeDatadogClient struct {
	queryMetricsFunc  func(from, to int64, query string) ([]datadog.Series, error)
	getRateLimitsFunc func() map[string]datadog.RateLimit
}

type fakeProcessor struct {
	updateMetricFunc func(emList map[string]custommetrics.ExternalMetricValue) (updated map[string]custommetrics.ExternalMetricValue)
	processFunc      func(metrics []custommetrics.ExternalMetricValue) map[string]custommetrics.ExternalMetricValue
}

func (h *fakeProcessor) UpdateExternalMetrics(emList map[string]custommetrics.ExternalMetricValue) (updated map[string]custommetrics.ExternalMetricValue) {
	if h.updateMetricFunc != nil {
		return h.updateMetricFunc(emList)
	}
	return nil
}

func (h *fakeProcessor) ProcessEMList(metrics []custommetrics.ExternalMetricValue) map[string]custommetrics.ExternalMetricValue {
	if h.processFunc != nil {
		return h.processFunc(metrics)
	}
	return nil
}

func (h *fakeProcessor) QueryExternalMetric(queries []string, timeWindow time.Duration) (map[string]autoscalers.Point, error) {
	return nil, nil
}

func (d *fakeDatadogClient) QueryMetrics(from, to int64, query string) ([]datadog.Series, error) {
	if d.queryMetricsFunc != nil {
		return d.queryMetricsFunc(from, to, query)
	}
	return nil, nil
}

func (d *fakeDatadogClient) GetRateLimitStats() map[string]datadog.RateLimit {
	if d.getRateLimitsFunc != nil {
		return d.getRateLimitsFunc()
	}
	return nil
}

var maxAge = 30 * time.Second

func makePoints(ts int, val float64) datadog.DataPoint {
	if ts == 0 {
		ts = (int(metav1.Now().Unix()) - int(maxAge.Seconds()/2)) * 1000 // use ms
	}
	tsPtr := float64(ts)
	return datadog.DataPoint{&tsPtr, &val}
}

func makeAnnotations(metricName string, labels map[string]string) map[string]string {
	return map[string]string{
		"kubectl.kubernetes.io/last-applied-configuration": fmt.Sprintf(`
			"kind":"HorizontalPodAutoscaler",
			"spec":{
				"metrics":[{
					"external":{
						"metricName":"%s",
						"metricSelector":{
							"matchLabels":{
								%s
							}
						},
					},
					"type":"External"
					}],
				}
			}"`, metricName, labels),
	}
}

// TestupdateExternalMetrics checks the reconciliation between the local cache and the global store logic
func TestUpdate(t *testing.T) {
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

	hctrl, _ := newFakeAutoscalerController(t, client, alwaysLeader, autoscalers.DatadogClient(d))
	hctrl.poller.refreshPeriod = 600
	hctrl.poller.gcPeriodSeconds = 600
	hctrl.autoscalers = make(chan interface{}, 1)
	foo := autoscalers.ProcessorInterface(p)
	hctrl.hpaProc = foo

	// Fresh start with no activity. Both the local cache and the Global Store are empty.
	hctrl.updateExternalMetrics()
	metrics, err := store.ListAllExternalMetricValues()
	require.NoError(t, err)
	require.Len(t, metrics.External, 0)

	// Start the DCA with already existing Data
	// Check if nothing in local store and Global Store is full we update the Global Store metrics correctly
	metricsToStore := map[string]custommetrics.ExternalMetricValue{
		"external_metric-horizontal-default-foo-metric1": {
			MetricName: "metric1",
			Labels:     map[string]string{"foo": "bar"},
			Ref: custommetrics.ObjectReference{
				Type:      "horizontal",
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
	require.Len(t, metrics.External, 1)
	hctrl.toStore.m.Lock()
	require.Len(t, hctrl.toStore.data, 0)
	hctrl.toStore.m.Unlock()

	hctrl.updateExternalMetrics()

	metrics, err = store.ListAllExternalMetricValues()
	require.NoError(t, err)
	require.Len(t, metrics.External, 1)
	hctrl.toStore.m.Lock()
	require.Len(t, hctrl.toStore.data, 0)
	hctrl.toStore.m.Unlock()

	// Fresh start
	// Check if local store is not empty
	hctrl.toStore.m.Lock()
	hctrl.toStore.data["external_metric-horizontal-default-foo-metric2"] = custommetrics.ExternalMetricValue{
		MetricName: "metric2",
		Labels:     map[string]string{"foo": "bar"},
		Ref: custommetrics.ObjectReference{
			Type:      "horizontal",
			Name:      "foo",
			Namespace: "default",
		},
	}
	require.Len(t, hctrl.toStore.data, 1)
	hctrl.toStore.m.Unlock()

	hctrl.updateExternalMetrics()
	metrics, err = store.ListAllExternalMetricValues()
	require.NoError(t, err)
	require.Len(t, metrics.External, 2)

	// DCA becomes leader
	// Check that if there is conflicting info from the local store and the Global Store that we merge correctly
	// Check conflict on metric name and labels
	hctrl.toStore.m.Lock()
	hctrl.toStore.data["external_metric-horizontal-default-foo-metric2"] = custommetrics.ExternalMetricValue{
		MetricName: "metric2",
		Labels:     map[string]string{"foo": "baz"},
		Ref: custommetrics.ObjectReference{
			Type:      "horizontal",
			Name:      "foo",
			Namespace: "default",
		},
	}
	require.Len(t, hctrl.toStore.data, 1)
	hctrl.toStore.m.Unlock()
	hctrl.updateExternalMetrics()
	metrics, err = store.ListAllExternalMetricValues()
	require.NoError(t, err)
	require.Len(t, metrics.External, 2)

	for _, m := range metrics.External {
		require.True(t, m.Valid)
		if m.MetricName == "metric2" {
			require.True(t, reflect.DeepEqual(m.Labels, map[string]string{"foo": "baz"}))
		}
	}
}

// TestAutoscalerController is an integration test of the AutoscalerController
func TestAutoscalerController(t *testing.T) {
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
			Scope: pointer.Ptr("foo:bar"),
		},
	}
	d := &fakeDatadogClient{
		queryMetricsFunc: func(from, to int64, query string) ([]datadog.Series, error) {
			return ddSeries, nil
		},
	}
	hctrl, inf := newFakeAutoscalerController(t, client, alwaysLeader, autoscalers.DatadogClient(d))
	hctrl.poller.refreshPeriod = 600
	hctrl.poller.gcPeriodSeconds = 600
	hctrl.autoscalers = make(chan interface{}, 1)

	stop := make(chan struct{})
	defer close(stop)
	inf.Start(stop)

	go hctrl.RunHPA(stop)

	hctrl.RunControllerLoop(stop)

	c := client.AutoscalingV2beta1()
	require.NotNil(t, c)

	mockedHPA := newFakeHorizontalPodAutoscaler(
		"hpa_1",
		"default",
		"1",
		"foo",
		map[string]string{"foo": "bar"},
	)
	mockedHPA.Annotations = makeAnnotations("foo", map[string]string{"foo": "bar"})

	_, err := c.HorizontalPodAutoscalers("default").Create(context.TODO(), mockedHPA, metav1.CreateOptions{})
	require.NoError(t, err)

	timeout := time.NewTimer(5 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	select {
	case key := <-hctrl.autoscalers:
		t.Logf("hctrl process key:%s", key)
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}

	// Check local cache store is 1:1 with expectations
	storedHPA, err := hctrl.autoscalersLister.ByNamespace(mockedHPA.Namespace).Get(mockedHPA.Name)
	require.NoError(t, err)
	require.Equal(t, storedHPA, mockedHPA)
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
		require.NotZero(t, len(storedExternal.External))
		require.Equal(t, storedExternal.External[0].Value, float64(14.123))
		require.Equal(t, storedExternal.External[0].Labels, map[string]string{"foo": "bar"})
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}

	// Update the Metrics
	mockedHPA.Spec.Metrics = []autoscalingv2.MetricSpec{
		{
			Type: autoscalingv2.ExternalMetricSourceType,
			External: &autoscalingv2.ExternalMetricSource{
				MetricName: "foo",
				MetricSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"dcos_version": "2.1.9",
					},
				},
			},
		},
	}
	ddSeries = []datadog.Series{
		{
			Metric: &metricName,
			Points: []datadog.DataPoint{
				makePoints(1531492452000, 12.34),
				makePoints(penTime, 1.01),
				makePoints(0, 0.902),
			},
			Scope: pointer.Ptr("dcos_version:2.1.9"),
		},
	}
	mockedHPA.Annotations = makeAnnotations("nginx.net.request_per_s", map[string]string{"dcos_version": "2.1.9"})
	_, err = c.HorizontalPodAutoscalers(mockedHPA.Namespace).Update(context.TODO(), mockedHPA, metav1.UpdateOptions{})
	require.NoError(t, err)
	select {
	case key := <-hctrl.autoscalers:
		t.Logf("hctrl process key:%s", key)
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}
	storedHPA, err = hctrl.autoscalersLister.ByNamespace(mockedHPA.Namespace).Get(mockedHPA.Name)
	require.NoError(t, err)
	require.Equal(t, storedHPA, mockedHPA)
	// Checking the local cache holds the correct Data.
	ExtVal := autoscalers.InspectHPA(storedHPA)
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
		require.NotZero(t, len(storedExternal.External))
		require.Equal(t, storedExternal.External[0].Value, float64(1.01))
		require.Equal(t, storedExternal.External[0].Labels, map[string]string{"dcos_version": "2.1.9"})
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}

	newMockedHPA := newFakeHorizontalPodAutoscaler(
		"hpa_2",
		"default",
		"1",
		"foo",
		map[string]string{"foo": "bar"},
	)
	mockedHPA.Annotations = makeAnnotations("foo", map[string]string{"foo": "bar"})

	_, err = c.HorizontalPodAutoscalers("default").Create(context.TODO(), newMockedHPA, metav1.CreateOptions{})
	require.NoError(t, err)
	select {
	case key := <-hctrl.autoscalers:
		t.Logf("hctrl process key:%s", key)
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}

	// Verify that a Delete removes the Data from the Global Store and decreases metricsProcessdCount
	err = c.HorizontalPodAutoscalers("default").Delete(context.TODO(), newMockedHPA.Name, metav1.DeleteOptions{})
	require.NoError(t, err)
	select {
	case <-ticker.C:

	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}
	// Verify that a Delete removes the Data from the Global Store
	err = c.HorizontalPodAutoscalers("default").Delete(context.TODO(), mockedHPA.Name, metav1.DeleteOptions{})
	require.NoError(t, err)
	select {
	case <-ticker.C:
		storedExternal, err := store.ListAllExternalMetricValues()
		require.NoError(t, err)
		require.Len(t, storedExternal.External, 0)
		hctrl.toStore.m.Lock()
		st := hctrl.toStore.data
		hctrl.toStore.m.Unlock()
		require.Len(t, st, 0)
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}
}

func TestAutoscalerSync(t *testing.T) {
	client := newClient()
	d := &fakeDatadogClient{}
	hctrl, inf := newFakeAutoscalerController(t, client, alwaysLeader, d)
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
	err = hctrl.syncHPA(key)
	require.NoError(t, err)

	fakeKey := "default/prometheus"
	err = hctrl.syncHPA(fakeKey)
	require.Error(t, err, errors.IsNotFound)
}

// TestAutoscalerControllerGC tests the GC process of of the controller
func TestAutoscalerControllerGC(t *testing.T) {
	testCases := []struct {
		caseName string
		metrics  map[string]custommetrics.ExternalMetricValue
		hpa      *autoscalingv2.HorizontalPodAutoscaler
		expected []custommetrics.ExternalMetricValue
	}{
		{
			caseName: "hpa exists for metric",
			metrics: map[string]custommetrics.ExternalMetricValue{
				"external_metric-horizontal-default-foo-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"bar": "baz"},
					Ref:        custommetrics.ObjectReference{Type: "horizontal", Name: "foo", Namespace: "default", UID: "1111"},
					Timestamp:  12,
					Value:      1,
					Valid:      false,
				},
			},
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
					UID:       "1111",
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "HorizontalPodAutoscaler",
					APIVersion: "v2beta1",
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
					Ref:        custommetrics.ObjectReference{Type: "horizontal", Name: "foo", Namespace: "default", UID: "1111"},
					Timestamp:  12,
					Value:      1,
					Valid:      false,
				},
			},
		},
		{
			caseName: "no hpa for metric",
			metrics: map[string]custommetrics.ExternalMetricValue{
				"external_metric-horizontal-default-foo-requests_per_s_b": {
					MetricName: "requests_per_s_b",
					Labels:     map[string]string{"bar": "baz"},
					Ref:        custommetrics.ObjectReference{Type: "horizontal", Name: "foo", Namespace: "default", UID: "1111"},
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
			d := &fakeDatadogClient{}
			hctrl, inf := newFakeAutoscalerController(t, client, alwaysLeader, d)

			hctrl.store = store

			if testCase.hpa != nil {
				genericInformer, err := inf.ForResource(autoscalingv2.SchemeGroupVersion.WithResource(hpaResource))
				require.NoError(t, err)

				err = genericInformer.Informer().GetStore().Add(testCase.hpa)
				require.NoError(t, err)
			}
			hctrl.gc() // force gc to run
			allMetrics, err := store.ListAllExternalMetricValues()
			require.NoError(t, err)
			assert.ElementsMatch(t, testCase.expected, allMetrics.External)
		})
	}
}

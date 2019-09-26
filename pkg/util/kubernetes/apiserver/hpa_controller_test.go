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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/zorkian/go-datadog-api.v2"
	"k8s.io/api/autoscaling/v2beta1"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hpa"
)

func newFakeConfigMapStore(t *testing.T, ns, name string, metrics map[string]custommetrics.ExternalMetricValue) (custommetrics.Store, kubernetes.Interface) {
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
	)
	ExtendToHPAController(autoscalerController, informerFactory.Autoscaling().V2beta1().HorizontalPodAutoscalers())

	autoscalerController.autoscalersListerSynced = func() bool { return true }
	autoscalerController.overFlowingHPAs = make(map[types.UID]int)

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

func (d *fakeDatadogClient) QueryMetrics(from, to int64, query string) ([]datadog.Series, error) {
	if d.queryMetricsFunc != nil {
		return d.queryMetricsFunc(from, to, query)
	}
	return nil, nil
}

var maxAge = time.Duration(30 * time.Second)

func makePoints(ts int, val float64) datadog.DataPoint {
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

	hctrl, _ := newFakeAutoscalerController(client, alwaysLeader, hpa.DatadogClient(d))
	hctrl.poller.refreshPeriod = 600
	hctrl.poller.gcPeriodSeconds = 600
	hctrl.autoscalers = make(chan interface{}, 1)
	foo := hpa.ProcessorInterface(p)
	hctrl.hpaProc = foo

	// Fresh start with no activity. Both the local cache and the Global Store are empty.
	hctrl.updateExternalMetrics()
	metrics, err := store.ListAllExternalMetricValues()
	require.NoError(t, err)
	require.Len(t, metrics, 0)

	// Start the DCA with already existing Data
	// Check if nothing in local store and Global Store is full we update the Global Store metrics correctly
	metricsToStore := map[string]custommetrics.ExternalMetricValue{
		"external_metric-default-foo-metric1": {
			MetricName: "metric1",
			Labels:     map[string]string{"foo": "bar"},
			Ref: custommetrics.ObjectReference{
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
	hctrl.toStore.data["external_metric-default-foo-metric2"] = custommetrics.ExternalMetricValue{
		MetricName: "metric2",
		Labels:     map[string]string{"foo": "bar"},
		Ref: custommetrics.ObjectReference{
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
	hctrl.toStore.data["external_metric-default-foo-metric2"] = custommetrics.ExternalMetricValue{
		MetricName: "metric2",
		Labels:     map[string]string{"foo": "baz"},
		Ref: custommetrics.ObjectReference{
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
	hctrl, inf := newFakeAutoscalerController(client, alwaysLeader, hpa.DatadogClient(d))
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
	require.Equal(t, hctrl.metricsProcessedCount, 0)

	mockedHPA := newFakeHorizontalPodAutoscaler(
		"hpa_1",
		"default",
		"1",
		"foo",
		map[string]string{"foo": "bar"},
	)
	mockedHPA.Annotations = makeAnnotations("foo", map[string]string{"foo": "bar"})

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
		require.Len(t, st, 1)
		require.Equal(t, hctrl.metricsProcessedCount, 1)

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
	mockedHPA.Annotations = makeAnnotations("nginx.net.request_per_s", map[string]string{"dcos_version": "2.1.9"})
	_, err = c.HorizontalPodAutoscalers(mockedHPA.Namespace).Update(mockedHPA)
	require.NoError(t, err)
	select {
	case <-hctrl.autoscalers:
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}
	require.Equal(t, hctrl.metricsProcessedCount, 1)
	storedHPA, err = hctrl.autoscalersLister.HorizontalPodAutoscalers(mockedHPA.Namespace).Get(mockedHPA.Name)
	require.NoError(t, err)
	require.Equal(t, storedHPA, mockedHPA)
	// Checking the local cache holds the correct Data.
	ExtVal := hpa.Inspect(storedHPA)
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

	// fake the ignoring
	hctrl.mu.Lock()
	hctrl.metricsProcessedCount = 45
	hctrl.mu.Unlock()

	_, err = c.HorizontalPodAutoscalers("default").Create(newMockedHPA)
	require.NoError(t, err)
	select {
	case <-hctrl.autoscalers:
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}
	require.Equal(t, hctrl.metricsProcessedCount, 45)
	require.Equal(t, hctrl.overFlowingHPAs[newMockedHPA.UID], 1)

	// Verify that a Delete removes the Data from the Global Store and decreases metricsProcessdCount
	err = c.HorizontalPodAutoscalers("default").Delete(newMockedHPA.Name, &metav1.DeleteOptions{})
	require.NoError(t, err)
	select {
	case <-ticker.C:

	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}
	hctrl.mu.Lock()
	require.Equal(t, hctrl.metricsProcessedCount, 45)
	require.Equal(t, len(hctrl.overFlowingHPAs), 0)
	hctrl.mu.Unlock()
	// Verify that a Delete removes the Data from the Global Store and decreases metricsProcessdCount at it was not ignored
	err = c.HorizontalPodAutoscalers("default").Delete(mockedHPA.Name, &metav1.DeleteOptions{})
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
	hctrl.mu.Lock()
	require.Equal(t, hctrl.metricsProcessedCount, 44)
	hctrl.mu.Unlock()

	_, err = c.HorizontalPodAutoscalers("default").Create(newMockedHPA)
	require.NoError(t, err)
	select {
	case <-hctrl.autoscalers:
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting for HPAs to update")
	}
	require.Equal(t, hctrl.metricsProcessedCount, 45)
	require.Equal(t, len(hctrl.overFlowingHPAs), 0)
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

	require.Empty(t, hctrl.overFlowingHPAs)
	hctrl.mu.Lock()
	hctrl.metricsProcessedCount = 44
	hctrl.mu.Unlock()
	ignoredHPA := newFakeHorizontalPodAutoscaler(
		"hpa_2",
		"default",
		"123",
		"foo",
		map[string]string{"foo": "bar"},
	)
	ignoredHPA.Spec.Metrics = append(ignoredHPA.Spec.Metrics, autoscalingv2.MetricSpec{
		Type: v2beta1.ExternalMetricSourceType,
		External: &v2beta1.ExternalMetricSource{
			MetricName: "deadbeef",
			MetricSelector: &metav1.LabelSelector{
				MatchLabels: nil,
			},
		},
	})
	err = inf.Autoscaling().V2beta1().HorizontalPodAutoscalers().Informer().GetStore().Add(ignoredHPA)
	require.NoError(t, err)
	keyToIgnore := "default/hpa_2"
	err = hctrl.syncAutoscalers(keyToIgnore)
	require.Nil(t, err)
	require.NotEmpty(t, hctrl.overFlowingHPAs)
	require.Equal(t, hctrl.overFlowingHPAs["123"], 2)
	require.Equal(t, hctrl.metricsProcessedCount, 44)
	hctrl.toStore.m.Lock()
	require.Equal(t, len(hctrl.toStore.data), 1)
	hctrl.toStore.m.Unlock()
}

func TestRemoveIgnoredHPAs(t *testing.T) {
	listToIgnore := map[types.UID]int{
		"aaa": 1,
		"bbb": 2,
	}
	cachedHPAs := []*autoscalingv2.HorizontalPodAutoscaler{
		{
			ObjectMeta: metav1.ObjectMeta{
				UID: "aaa",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				UID: "ccc",
			},
		},
	}

	e := removeIgnoredHPAs(listToIgnore, cachedHPAs)
	require.Equal(t, len(e), 1)
	require.Equal(t, e[0].UID, types.UID("ccc"))

}

// TestAutoscalerControllerGC tests the GC process of of the controller
func TestAutoscalerControllerGC(t *testing.T) {
	testCases := []struct {
		caseName string
		metrics  map[string]custommetrics.ExternalMetricValue
		hpa      *v2beta1.HorizontalPodAutoscaler
		ignored  map[types.UID]int
		expected []custommetrics.ExternalMetricValue
	}{
		{
			caseName: "hpa exists for metric",
			metrics: map[string]custommetrics.ExternalMetricValue{
				"external_metric-default-foo-requests_per_s": {
					MetricName: "requests_per_s",
					Labels:     map[string]string{"bar": "baz"},
					Ref:        custommetrics.ObjectReference{Name: "foo", Namespace: "default", UID: "1111"},
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
			ignored: map[types.UID]int{},
			expected: []custommetrics.ExternalMetricValue{ // skipped by gc
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"bar": "baz"},
					Ref:        custommetrics.ObjectReference{Name: "foo", Namespace: "default", UID: "1111"},
					Timestamp:  12,
					Value:      1,
					Valid:      false,
				},
			},
		},
		{
			caseName: "no hpa for metric",
			metrics: map[string]custommetrics.ExternalMetricValue{
				"external_metric-default-foo-requests_per_s_b": {
					MetricName: "requests_per_s_b",
					Labels:     map[string]string{"bar": "baz"},
					Ref:        custommetrics.ObjectReference{Name: "foo", Namespace: "default", UID: "1111"},
					Timestamp:  12,
					Value:      1,
					Valid:      false,
				},
			},
			ignored:  map[types.UID]int{},
			expected: []custommetrics.ExternalMetricValue{},
		},
		{
			caseName: "hpa in global store but is ignored need to remove",
			metrics: map[string]custommetrics.ExternalMetricValue{
				"external_metric-default-foo-requests_per_s": {
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
			ignored: map[types.UID]int{
				"1111": 1,
			},
			expected: []custommetrics.ExternalMetricValue{},
		},
		{
			// For this test case, we don't see a difference, as the hpa is dropped before getting to DiffExternalMetrics
			caseName: "hpa not in global store but ignored",
			metrics:  map[string]custommetrics.ExternalMetricValue{},
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
			ignored: map[types.UID]int{
				"1111": 1,
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
			hctrl.overFlowingHPAs = testCase.ignored

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

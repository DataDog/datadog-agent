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
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/zorkian/go-datadog-api.v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	wpa_informers "k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	fake_k "k8s.io/client-go/kubernetes/fake"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	"github.com/DataDog/watermarkpodautoscaler/api/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

var (
	scheme = kscheme.Scheme
)

func init() {
	v1alpha1.AddToScheme(scheme)
}

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
	require.Len(t, metrics.External, 2)

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
	require.Len(t, metrics.External, 2)

	for _, m := range metrics.External {
		require.True(t, m.Valid)
		if m.MetricName == "metric2" {
			require.True(t, reflect.DeepEqual(m.Labels, map[string]string{"foo": "baz"}))
		}
	}

}

// newFakeWPAController creates an AutoscalersController. Use enableWPA(wpa_informers.SharedInformerFactory) to add the event handlers to it. Use Run() to add the event handlers and start processing the events.
func newFakeWPAController(t *testing.T, kubeClient kubernetes.Interface, client dynamic.Interface, isLeaderFunc func() bool, dcl autoscalers.DatadogClient) (*AutoscalersController, wpa_informers.DynamicSharedInformerFactory) {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(t.Logf)

	// need to fake wpa_client.
	inf := wpa_informers.NewDynamicSharedInformerFactory(client, 0)
	autoscalerController, _ := NewAutoscalersController(
		kubeClient,
		eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "FakeWPAController"}),
		isLeaderFunc,
		dcl,
	)

	autoscalerController.autoscalersListerSynced = func() bool { return true }

	return autoscalerController, inf
}

func newFakeWatermarkPodAutoscaler(name, ns string, uid types.UID, metricName string, labels map[string]string) (obj *unstructured.Unstructured, wpa *v1alpha1.WatermarkPodAutoscaler) {
	obj = &unstructured.Unstructured{}
	wpa = &v1alpha1.WatermarkPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "WatermarkPodAutoscaler",
			APIVersion: "datadoghq.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			UID:       uid,
		},
		Spec: v1alpha1.WatermarkPodAutoscalerSpec{
			Metrics: []v1alpha1.MetricSpec{
				{
					Type: v1alpha1.ExternalMetricSourceType,
					External: &v1alpha1.ExternalMetricSource{
						MetricName: metricName,
						MetricSelector: &metav1.LabelSelector{
							MatchLabels: labels,
						},
					},
				},
			},
		},
	}

	if err := UnstructuredFromAutoscaler(wpa, obj); err != nil {
		panic("Failed to construct unstructured WPA")
	}

	return
}

// TestWPAController is an integration test of the AutoscalerController
func TestWPAController(t *testing.T) {
	logFlush := configureLoggerForTest(t)
	defer logFlush()
	metricName := "foo"
	namespace := "default"
	wpaName := "wpa_1"

	penTime := (int(time.Now().Unix()) - int(maxAge.Seconds()/2)) * 1000
	name := custommetrics.GetConfigmapName()
	store, client := newFakeConfigMapStore(t, namespace, name, nil)
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
	d := &fakeDatadogClient{
		queryMetricsFunc: func(from, to int64, query string) ([]datadog.Series, error) {
			return ddSeries, nil
		},
	}

	_, mockedWPA := newFakeWatermarkPodAutoscaler(
		wpaName,
		namespace,
		"1",
		metricName,
		map[string]string{"foo": "bar"},
	)

	wpaClient := fake.NewSimpleDynamicClient(scheme, mockedWPA)

	hctrl, inf := newFakeWPAController(t, client, wpaClient, alwaysLeader, autoscalers.DatadogClient(d))
	hctrl.poller.refreshPeriod = 600
	hctrl.poller.gcPeriodSeconds = 600
	hctrl.autoscalers = make(chan interface{}, 1)

	stop := make(chan struct{})
	defer close(stop)
	inf.WaitForCacheSync(stop)
	inf.Start(stop)

	go hctrl.RunWPA(stop, wpaClient, inf)

	hctrl.RunControllerLoop(stop)

	res, err := wpaClient.Resource(gvrWPA).Namespace(namespace).Get(context.TODO(), wpaName, v1.GetOptions{})
	require.NoError(t, err)

	wpaDecoded := &v1alpha1.WatermarkPodAutoscaler{}
	err = UnstructuredIntoWPA(res, wpaDecoded)
	require.NoError(t, err)
	require.Equal(t, wpaName, wpaDecoded.Name)

	timeout := 5 * time.Second
	frequency := 500 * time.Millisecond

	testutil.RequireTrueBeforeTimeout(t, frequency, timeout, func() bool {
		key := <-hctrl.autoscalers
		t.Logf("hctrl process key:%s", key)
		return true
	})
	// Check local cache store is 1:1 with expectations
	storedWPAObject, err := hctrl.wpaLister.ByNamespace(namespace).Get(wpaName)
	require.NoError(t, err)

	storedWPA := &v1alpha1.WatermarkPodAutoscaler{}
	err = UnstructuredIntoWPA(storedWPAObject, storedWPA)
	require.NoError(t, err)

	require.Equal(t, storedWPA, wpaDecoded)
	testutil.RequireTrueBeforeTimeout(t, frequency, timeout, func() bool {
		hctrl.toStore.m.Lock()
		st := hctrl.toStore.data
		hctrl.toStore.m.Unlock()
		return len(st) == 1
	})

	hctrl.updateExternalMetrics()

	// Test that the Global store contains the correct data
	testutil.RequireTrueBeforeTimeout(t, frequency, timeout, func() bool {
		storedExternal, err := store.ListAllExternalMetricValues()
		require.NoError(t, err)
		if len(storedExternal.External) == 0 {
			return false
		}
		require.Equal(t, storedExternal.External[0].Value, float64(14.123))
		require.Equal(t, storedExternal.External[0].Labels, map[string]string{"foo": "bar"})
		return true
	})

	retrier := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		resWPA, errWPA := wpaClient.Resource(gvrWPA).Namespace(namespace).Get(context.TODO(), wpaName, v1.GetOptions{})
		require.NoError(t, errWPA)

		metrics, found, err := unstructured.NestedSlice(resWPA.Object, "spec", "metrics")
		require.Equal(t, found, true)
		require.NoError(t, err)

		ext, found, err := unstructured.NestedMap(metrics[0].(map[string]interface{}), "external")
		require.Equal(t, found, true)
		require.NoError(t, err)
		matchLabels, found, err := unstructured.NestedMap(ext, "metricSelector")
		require.Equal(t, found, true)
		require.NoError(t, err)

		err = unstructured.SetNestedField(matchLabels, map[string]interface{}{"dcos_version": "2.1.9"}, "matchLabels")
		require.NoError(t, err)

		err = unstructured.SetNestedField(ext, matchLabels, "metricSelector")
		require.NoError(t, err)

		err = unstructured.SetNestedField(metrics[0].(map[string]interface{}), ext, "external")
		require.NoError(t, err)

		err = unstructured.SetNestedField(resWPA.Object, metrics, "spec", "metrics")
		require.NoError(t, err)

		annotations := makeAnnotations("nginx.net.request_per_s", map[string]string{"dcos_version": "2.1.9"})
		err = unstructured.SetNestedField(resWPA.Object, annotations["kubectl.kubernetes.io/last-applied-configuration"], "metadata", "annotations", "kubectl.kubernetes.io/last-applied-configuration")
		require.NoError(t, err)

		res, updateErr := wpaClient.Resource(gvrWPA).Namespace(namespace).Update(context.TODO(), resWPA, v1.UpdateOptions{})
		err = UnstructuredIntoWPA(res, wpaDecoded)
		require.NoError(t, err)

		return updateErr
	})
	require.NoError(t, retrier)

	// Update the Metrics
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

	testutil.RequireTrueBeforeTimeout(t, frequency, timeout, func() bool {
		key := <-hctrl.autoscalers
		t.Logf("hctrl process key:%s", key)
		return true
	})

	storedWPAObject, err = hctrl.wpaLister.ByNamespace(namespace).Get(wpaName)
	require.NoError(t, err)
	err = UnstructuredIntoWPA(storedWPAObject, storedWPA)
	require.NoError(t, err)
	require.Equal(t, storedWPA, wpaDecoded)
	// Checking the local cache holds the correct Data.
	ExtVal := autoscalers.InspectWPA(storedWPA)
	key := custommetrics.ExternalMetricValueKeyFunc(ExtVal[0])

	// Process and submit to the Global Store
	testutil.RequireTrueBeforeTimeout(t, frequency, timeout, func() bool {
		hctrl.toStore.m.Lock()
		defer hctrl.toStore.m.Unlock()
		st := hctrl.toStore.data
		if len(st) == 0 {
			return false
		}
		require.Len(t, st, 1)
		// Not comparing timestamps to avoid flakyness.
		require.Equal(t, ExtVal[0].Ref, st[key].Ref)
		require.Equal(t, ExtVal[0].MetricName, st[key].MetricName)
		require.Equal(t, ExtVal[0].Labels, st[key].Labels)
		return true
	})

	hctrl.updateExternalMetrics()

	testutil.RequireTrueBeforeTimeout(t, frequency, timeout, func() bool {
		storedExternal, err := store.ListAllExternalMetricValues()
		require.NoError(t, err)
		if len(storedExternal.External) == 0 {
			return false
		}
		require.Equal(t, float64(1.01), storedExternal.External[0].Value)
		require.Equal(t, map[string]string{"dcos_version": "2.1.9"}, storedExternal.External[0].Labels)
		return true
	})

	// Verify that a Delete removes the Data from the Global Store
	err = wpaClient.Resource(gvrWPA).Namespace(namespace).Delete(context.TODO(), wpaName, v1.DeleteOptions{})
	require.NoError(t, err)
	testutil.RequireTrueBeforeTimeout(t, frequency, timeout, func() bool {
		storedExternal, err := store.ListAllExternalMetricValues()
		require.NoError(t, err)
		if len(storedExternal.External) != 0 {
			return false
		}
		hctrl.toStore.m.Lock()
		defer hctrl.toStore.m.Unlock()
		require.Len(t, hctrl.toStore.data, 0)
		return true
	})
}

// TestWPASync tests the sync loop of the informer cache and the processing of the object
func TestWPASync(t *testing.T) {
	wpaClient := fake.NewSimpleDynamicClient(scheme)
	client := fake_k.NewSimpleClientset()
	d := &fakeDatadogClient{}
	hctrl, inf := newFakeWPAController(t, client, wpaClient, alwaysLeader, d)
	hctrl.enableWPA(inf)
	obj, _ := newFakeWatermarkPodAutoscaler(
		"wpa_1",
		"default",
		"1",
		"foo",
		map[string]string{"foo": "bar"},
	)

	err := inf.ForResource(gvrWPA).Informer().GetStore().Add(obj)
	require.NoError(t, err)
	key := "default/wpa_1"
	err = hctrl.syncWPA(key)
	require.NoError(t, err)

	fakeKey := "default/prometheus"
	err = hctrl.syncWPA(fakeKey)
	require.Error(t, err, errors.IsNotFound)

}

// TestWPAGC tests the GC process of of the controller
func TestWPAGC(t *testing.T) {
	wpaFoo, _ := newFakeWatermarkPodAutoscaler("foo", "default", "1111", "requests_per_s", map[string]string{"bar": "baz"})
	testCases := []struct {
		caseName string
		metrics  map[string]custommetrics.ExternalMetricValue
		wpa      *unstructured.Unstructured
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
			wpa: wpaFoo,
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
		{
			caseName: "wpa in global store but is ignored need to remove",
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
			wpa:      wpaFoo,
			expected: []custommetrics.ExternalMetricValue{},
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("#%d %s", i, testCase.caseName), func(t *testing.T) {
			store, client := newFakeConfigMapStore(t, "default", fmt.Sprintf("test-%d", i), testCase.metrics)
			d := &fakeDatadogClient{}
			wpaCl := fake.NewSimpleDynamicClient(scheme)

			hctrl, _ := newFakeAutoscalerController(t, client, alwaysLeader, d)
			hctrl.wpaEnabled = true
			inf := wpa_informers.NewDynamicSharedInformerFactory(wpaCl, 0)
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			inf.WaitForCacheSync(ctx.Done())
			hctrl.enableWPA(inf)

			hctrl.store = store

			if testCase.wpa != nil {
				err := inf.ForResource(gvrWPA).
					Informer().
					GetStore().
					Add(testCase.wpa)
				require.NoError(t, err)
			}
			hctrl.gc() // force gc to run
			allMetrics, err := store.ListAllExternalMetricValues()
			require.NoError(t, err)
			assert.ElementsMatch(t, testCase.expected, allMetrics.External)
		})
	}
}

// TestUnstructuredIntoWPA test the conversion of unstructured object into WPA
func TestUnstructuredIntoWPA(t *testing.T) {
	wpa1, _ := newFakeWatermarkPodAutoscaler("wpa-1", "ns", "1234-abc", "test", map[string]string{"foo": "bar"})
	testCases := []struct {
		caseName    string
		obj         interface{}
		expectedWpa *v1alpha1.WatermarkPodAutoscaler
		error       error
	}{
		{
			caseName:    "obj corrupted",
			obj:         map[string]interface{}{},
			expectedWpa: nil,
			error:       fmt.Errorf("could not cast Unstructured object: map[]"),
		},
		{
			caseName: "All good",
			obj:      wpa1,
			expectedWpa: &v1alpha1.WatermarkPodAutoscaler{
				ObjectMeta: v1.ObjectMeta{
					Name:      "wpa-1",
					Namespace: "ns",
					UID:       "1234-abc",
				},
				Spec: v1alpha1.WatermarkPodAutoscalerSpec{
					Tolerance: resource.MustParse("0"),
					Metrics: []v1alpha1.MetricSpec{
						{
							Type: v1alpha1.ExternalMetricSourceType,
							External: &v1alpha1.ExternalMetricSource{
								MetricName:     "test",
								MetricSelector: &v1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
							},
						},
					},
				},
			},
			error: nil,
		},
	}
	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("#%d %s", i, testCase.caseName), func(t *testing.T) {
			testWPA := &v1alpha1.WatermarkPodAutoscaler{}
			err := UnstructuredIntoWPA(testCase.obj, testWPA)
			require.Equal(t, testCase.error, err)
			if err == nil {
				// because we use the fake client, the GVK is missing from the WPA object.
				require.Equal(t, testCase.expectedWpa.GetObjectMeta(), testWPA.GetObjectMeta())
				require.Equal(t, testCase.expectedWpa.Spec, testWPA.Spec)
			}
		})
	}

}

func TestWPACRDCheck(t *testing.T) {
	retryableError := apierrors.NewNotFound(schema.GroupResource{
		Group:    "datadoghq.com",
		Resource: "watermarkpodautoscalers",
	}, "")
	nonRetryableError := fmt.Errorf("unexpectedError")
	testCases := []struct {
		caseName      string
		checkError    error
		expectedError error
	}{
		{
			caseName:   "wpa crd exists",
			checkError: nil,
		},
		{
			caseName:      "wpa crd not found",
			checkError:    retryableError,
			expectedError: retryableError,
		},
		{
			caseName:      "wpa list non-retryable",
			checkError:    nonRetryableError,
			expectedError: backoff.Permanent(nonRetryableError),
		},
	}
	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("#%d %s", i, testCase.caseName), func(t *testing.T) {
			actualError := tryCheckWPACRD(func() error { return testCase.checkError })
			require.Equal(t, testCase.expectedError, actualError)
		})
	}
}

func configureLoggerForTest(t *testing.T) func() {
	logger, err := seelog.LoggerFromWriterWithMinLevel(testWriter{t}, seelog.TraceLvl)
	if err != nil {
		t.Fatalf("unable to configure logger, err: %v", err)
	}
	seelog.ReplaceLogger(logger) //nolint:errcheck
	log.SetupLogger(logger, "trace")
	return log.Flush
}

type testWriter struct {
	t *testing.T
}

func (tw testWriter) Write(p []byte) (n int, err error) {
	line := string(p)
	line = strings.TrimRight(line, "\r\n")
	tw.t.Log(line)
	return len(p), nil
}

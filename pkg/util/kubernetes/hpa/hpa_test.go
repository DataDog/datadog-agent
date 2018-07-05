// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newMockConfigMap(hpaName string, hpaNamespace string, metricName string, labels map[string]string) *v1.ConfigMap {
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      datadogHPAConfigMap,
			Namespace: "default",
		},
	}

	if metricName == "" || len(labels) == 0 {
		return cm
	}

	custMetric := CustomExternalMetric{
		Name:         metricName,
		Labels:       labels,
		Timestamp:    12,
		HPAName:      hpaName,
		HPANamespace: hpaNamespace,
		Value:        1,
		Valid:        false,
	}
	cm.Data = make(map[string]string)
	marsh, _ := json.Marshal(custMetric)
	key := fmt.Sprintf("external.metrics.%s.%s-%s", hpaNamespace, hpaName, metricName)
	cm.Data[key] = string(marsh)
	return cm
}

func newMockHPAExternalManifest(hpaName string, hpaNamespace string, metricName string, labels map[string]string) *v2beta1.HorizontalPodAutoscaler {
	return &v2beta1.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hpaName,
			Namespace: hpaNamespace,
		},
		Spec: v2beta1.HorizontalPodAutoscalerSpec{
			Metrics: []v2beta1.MetricSpec{
				{
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

func TestRemoveEntryFromConfigMap(t *testing.T) {
	hpaCl := HPAWatcherClient{
		clientSet: fake.NewSimpleClientset(),
		ns:        "default",
	}
	_, err := hpaCl.clientSet.CoreV1().ConfigMaps("default").Get(datadogHPAConfigMap, metav1.GetOptions{})
	fmt.Printf("err is %s \n", err)
	assert.True(t, errors.IsNotFound(err))

	testCases := []struct {
		caseName          string
		configmap         *v1.ConfigMap
		hpa               *v2beta1.HorizontalPodAutoscaler
		expectedConfigMap map[string]string
	}{
		{
			caseName:          "Metric exists, deleting",
			configmap:         newMockConfigMap("foohpa", "default", "foo", map[string]string{"bar": "baz"}),
			hpa:               newMockHPAExternalManifest("foohpa", "default", "foo", map[string]string{"bar": "baz"}),
			expectedConfigMap: map[string]string{},
		},
		{
			caseName:          "Metric is not listed, no-op",
			configmap:         newMockConfigMap("foohpa", "default", "foo", map[string]string{"bar": "baz"}),
			hpa:               newMockHPAExternalManifest("foohpa", "default", "bar", map[string]string{"bar": "baz"}),
			expectedConfigMap: map[string]string{"external.metrics.default.foohpa-foo": "{\"name\":\"foo\",\"labels\":{\"bar\":\"baz\"},\"ts\":12,\"hpa_name\":\"foohpa\",\"hpa_namespace\":\"default\",\"value\":1,\"valid\":false}"},
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("#%d %s", i, testCase.caseName), func(t *testing.T) {
			hpaCl.clientSet.CoreV1().ConfigMaps("default").Create(testCase.configmap)

			hpaCl.removeEntryFromConfigMap([]*v2beta1.HorizontalPodAutoscaler{testCase.hpa})
			cm, _ := hpaCl.clientSet.CoreV1().ConfigMaps("default").Get(datadogHPAConfigMap, metav1.GetOptions{})
			assert.Equal(t, testCase.expectedConfigMap, cm.Data)

			hpaCl.clientSet.CoreV1().ConfigMaps("default").Delete(datadogHPAConfigMap, &metav1.DeleteOptions{})
		})
	}

}

func newMockCustomExternalMetric(hpaName string, hpaNamespace string, metricName string, labels map[string]string) CustomExternalMetric {
	return CustomExternalMetric{
		Name:         metricName,
		Labels:       labels,
		Timestamp:    12,
		HPAName:      hpaName,
		HPANamespace: hpaNamespace,
		Value:        1,
	}
}

func TestReadConfigMap(t *testing.T) {
	hpaCl := HPAWatcherClient{
		clientSet: fake.NewSimpleClientset(),
		ns:        "default",
	}
	_, err := hpaCl.clientSet.CoreV1().ConfigMaps("default").Get(datadogHPAConfigMap, metav1.GetOptions{})
	assert.Contains(t, err.Error(), "configmaps \"datadog-hpa\" not found")

	testCases := []struct {
		caseName       string
		configmap      *v1.ConfigMap
		expectedResult []CustomExternalMetric
	}{
		{
			caseName:       "No correct metrics",
			configmap:      newMockConfigMap("foohpa", "default", "foo", map[string]string{}),
			expectedResult: nil,
		},
		{
			caseName:       "Metric has the expected format",
			configmap:      newMockConfigMap("foohpa", "default", "foo", map[string]string{"bar": "baz"}),
			expectedResult: []CustomExternalMetric{newMockCustomExternalMetric("foohpa", "default", "foo", map[string]string{"bar": "baz"})},
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("#%d %s", i, testCase.caseName), func(t *testing.T) {
			_, err = hpaCl.clientSet.CoreV1().ConfigMaps("default").Create(testCase.configmap)
			cmRead := hpaCl.ReadConfigMap()
			assert.Equal(t, testCase.expectedResult, cmRead)

			hpaCl.clientSet.CoreV1().ConfigMaps("default").Delete(datadogHPAConfigMap, &metav1.DeleteOptions{})
		})
	}
}

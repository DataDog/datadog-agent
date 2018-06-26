package hpa

import (
	"testing"

	"k8s.io/client-go/kubernetes/fake"
	//fake2 "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"encoding/json"
	"fmt"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/core/v1"
)

func newMockConfigMap(metricName string, labels map[string]string) *v1.ConfigMap {
	cm := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind: "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      datadogHPAConfigMap,
			Namespace: "default",
		},
	}
	if metricName == "" || len(labels) == 0 {
		return cm
	}

	custMetric := CustomExternalMetric{
		Name:      metricName,
		Labels:    labels,
		Timestamp: 12,
		HpaName:   "foo",
		Value:     1,
	}
	cm.Data = make(map[string]string)
	marsh, _ := json.Marshal(custMetric)
	cm.Data[metricName] = string(marsh)
	return cm
}

func newMockHPAExternalManifest(metricName string, labels map[string]string) *v2beta1.HorizontalPodAutoscaler {
	return &v2beta1.HorizontalPodAutoscaler{
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
	assert.Contains(t, err.Error(), "configmaps \"datadog-hpa\" not found")

	testCases := []struct {
		caseName          string
		configmap         *v1.ConfigMap
		hpa               *v2beta1.HorizontalPodAutoscaler
		expectedConfigMap map[string]string
	}{
		{
			caseName:          "Metric exists, deleting",
			configmap:         newMockConfigMap("foo", map[string]string{"bar": "baz"}),
			hpa:               newMockHPAExternalManifest("foo", map[string]string{"bar": "baz"}),
			expectedConfigMap: map[string]string{},
		},
		{
			caseName:          "Metric is not listed, no-op",
			configmap:         newMockConfigMap("foobar", map[string]string{"bar": "baz"}),
			hpa:               newMockHPAExternalManifest("foo", map[string]string{"bar": "baz"}),
			expectedConfigMap: map[string]string{"foobar": "{\"name\":\"foobar\",\"labels\":{\"bar\":\"baz\"},\"ts\":12,\"origin\":\"foo\",\"value\":1}"},
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

func newMockCustomExternalMetric(name string, labels map[string]string) CustomExternalMetric {
	return CustomExternalMetric{
		Name:      name,
		Labels:    labels,
		Timestamp: 12,
		HpaName:   "foo",
		Value:     1,
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
			configmap:      newMockConfigMap("foo", map[string]string{}),
			expectedResult: nil,
		},
		{
			caseName:       "Metric has the expected format",
			configmap:      newMockConfigMap("foo", map[string]string{"bar": "baz"}),
			expectedResult: []CustomExternalMetric{newMockCustomExternalMetric("foo", map[string]string{"bar": "baz"})},
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

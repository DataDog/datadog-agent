// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package podcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConvertToADConfig_Basic(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-check",
			Namespace: "web-team",
		},
		Spec: datadoghq.DatadogPodCheckSpec{
			ContainerImage: "nginx:latest",
			Check: datadoghq.CheckConfig{
				Name:       "nginx",
				InitConfig: &apiextensionsv1.JSON{Raw: []byte(`{}`)},
				Instances: []apiextensionsv1.JSON{
					{Raw: []byte(`{"nginx_status_url":"http://%%host%%:81/status/"}`)},
				},
			},
		},
	}

	yamlBytes, err := convertToADConfig(dpc)
	require.NoError(t, err)

	yaml := string(yamlBytes)
	assert.Contains(t, yaml, "ad_identifiers:\n- nginx:latest")
	assert.Contains(t, yaml, "init_config: {}")
	assert.Contains(t, yaml, "nginx_status_url: http://%%host%%:81/status/")
	assert.NotContains(t, yaml, "cel_selector")
	assert.NotContains(t, yaml, "logs:")
}

func TestConvertToADConfig_NilInitConfig(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-check",
			Namespace: "default",
		},
		Spec: datadoghq.DatadogPodCheckSpec{
			ContainerImage: "myapp",
			Check: datadoghq.CheckConfig{
				Name: "http_check",
				Instances: []apiextensionsv1.JSON{
					{Raw: []byte(`{"url":"http://localhost:8080"}`)},
				},
			},
		},
	}

	yamlBytes, err := convertToADConfig(dpc)
	require.NoError(t, err)

	yaml := string(yamlBytes)
	assert.Contains(t, yaml, "ad_identifiers:\n- myapp")
	assert.Contains(t, yaml, "init_config: null")
	assert.Contains(t, yaml, "url: http://localhost:8080")
}

func TestConvertToADConfig_MultipleInstances(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis-check",
			Namespace: "data-team",
		},
		Spec: datadoghq.DatadogPodCheckSpec{
			ContainerImage: "redis:7",
			Check: datadoghq.CheckConfig{
				Name:       "redisdb",
				InitConfig: &apiextensionsv1.JSON{Raw: []byte(`{}`)},
				Instances: []apiextensionsv1.JSON{
					{Raw: []byte(`{"host":"%%host%%","port":"6379"}`)},
					{Raw: []byte(`{"host":"%%host%%","port":"6380"}`)},
				},
			},
		},
	}

	yamlBytes, err := convertToADConfig(dpc)
	require.NoError(t, err)

	yaml := string(yamlBytes)
	assert.Contains(t, yaml, "ad_identifiers:\n- redis:7")
	assert.Contains(t, yaml, "port: \"6379\"")
	assert.Contains(t, yaml, "port: \"6380\"")
}

func TestConvertToADConfig_WithLogs(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-check",
			Namespace: "default",
		},
		Spec: datadoghq.DatadogPodCheckSpec{
			ContainerImage: "myapp:v1",
			Check: datadoghq.CheckConfig{
				Name: "http_check",
				Instances: []apiextensionsv1.JSON{
					{Raw: []byte(`{"url":"http://localhost"}`)},
				},
			},
			Logs: []apiextensionsv1.JSON{
				{Raw: []byte(`{"type":"file","path":"/var/log/app.log","service":"myapp"}`)},
			},
		},
	}

	yamlBytes, err := convertToADConfig(dpc)
	require.NoError(t, err)

	yaml := string(yamlBytes)
	assert.Contains(t, yaml, "logs:")
	assert.Contains(t, yaml, "service: myapp")
}

func TestConvertToADConfig_WithAnnotationSelector(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-check",
			Namespace: "web-team",
		},
		Spec: datadoghq.DatadogPodCheckSpec{
			ContainerImage: "nginx",
			Selector: &datadoghq.PodSelector{
				MatchAnnotations: map[string]string{
					"team": "web",
					"env":  "prod",
				},
			},
			Check: datadoghq.CheckConfig{
				Name: "nginx",
				Instances: []apiextensionsv1.JSON{
					{Raw: []byte(`{"nginx_status_url":"http://%%host%%/status/"}`)},
				},
			},
		},
	}

	yamlBytes, err := convertToADConfig(dpc)
	require.NoError(t, err)

	yaml := string(yamlBytes)
	assert.Contains(t, yaml, "cel_selector:")
	assert.Contains(t, yaml, `pod.annotations["env"] == "prod"`)
	assert.Contains(t, yaml, `pod.annotations["team"] == "web"`)
}

func TestConvertToADConfig_SelectorWithLabelsOnly(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-check",
			Namespace: "default",
		},
		Spec: datadoghq.DatadogPodCheckSpec{
			ContainerImage: "nginx",
			Selector: &datadoghq.PodSelector{
				MatchLabels: map[string]string{"app": "nginx"},
			},
			Check: datadoghq.CheckConfig{
				Name: "nginx",
				Instances: []apiextensionsv1.JSON{
					{Raw: []byte(`{"url":"http://%%host%%"}`)},
				},
			},
		},
	}

	yamlBytes, err := convertToADConfig(dpc)
	require.NoError(t, err)

	// matchLabels are deferred; no cel_selector should be generated
	yaml := string(yamlBytes)
	assert.NotContains(t, yaml, "cel_selector")
}

func TestConfigMapKey(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-check",
			Namespace: "web-team",
		},
		Spec: datadoghq.DatadogPodCheckSpec{
			Check: datadoghq.CheckConfig{Name: "nginx"},
		},
	}

	assert.Equal(t, "web-team_nginx-check_nginx.yaml", configMapKey(dpc))
}

func TestBuildAnnotationCELRules(t *testing.T) {
	rules := buildAnnotationCELRules(map[string]string{
		"b-key": "val-b",
		"a-key": "val-a",
	})

	require.Len(t, rules, 2)
	// Should be sorted by key
	assert.Equal(t, `pod.annotations["a-key"] == "val-a"`, rules[0])
	assert.Equal(t, `pod.annotations["b-key"] == "val-b"`, rules[1])
}

func TestConvertToADConfig_InvalidJSON(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bad-check",
			Namespace: "default",
		},
		Spec: datadoghq.DatadogPodCheckSpec{
			ContainerImage: "myapp",
			Check: datadoghq.CheckConfig{
				Name:       "test",
				InitConfig: &apiextensionsv1.JSON{Raw: []byte(`not-json`)},
				Instances: []apiextensionsv1.JSON{
					{Raw: []byte(`{"ok": true}`)},
				},
			},
		},
	}

	_, err := convertToADConfig(dpc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "initConfig")
}

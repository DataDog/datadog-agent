// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package checks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConvertCR_SingleCheck(t *testing.T) {
	dwc := &datadoghq.DatadogInstrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-check",
			Namespace: "web-team",
		},
		Spec: datadoghq.DatadogInstrumentationSpec{
			Selector: datadoghq.PodSelector{
				MatchLabels: map[string]string{"app": "nginx"},
			},
			Config: datadoghq.InstrumentationConfig{
				Checks: []datadoghq.CheckConfig{
					{
						Integration:    "nginx",
						ContainerImage: []string{"nginx:latest"},
						InitConfig:     &apiextensionsv1.JSON{Raw: []byte(`{}`)},
						Instances: []apiextensionsv1.JSON{
							{Raw: []byte(`{"nginx_status_url":"http://%%host%%:81/status/"}`)},
						},
					},
				},
			},
		},
	}

	entries, err := convertCR(dwc)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	yaml := entries["web-team_nginx-check_nginx.yaml"]
	assert.Contains(t, yaml, "ad_identifiers:\n- nginx:latest")
	assert.Contains(t, yaml, "init_config: {}")
	assert.Contains(t, yaml, "nginx_status_url")
	assert.Contains(t, yaml, `container.pod.labels["app"] == "nginx"`)
}

func TestConvertCR_MultipleChecks(t *testing.T) {
	dwc := &datadoghq.DatadogInstrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-check",
			Namespace: "default",
		},
		Spec: datadoghq.DatadogInstrumentationSpec{
			Selector: datadoghq.PodSelector{
				MatchLabels: map[string]string{"app": "myapp"},
			},
			Config: datadoghq.InstrumentationConfig{
				Checks: []datadoghq.CheckConfig{
					{
						Integration:    "http_check",
						ContainerImage: []string{"myapp:v1"},
						Instances:      []apiextensionsv1.JSON{{Raw: []byte(`{"url":"http://%%host%%:8080"}`)}},
					},
					{
						Integration:    "redisdb",
						ContainerImage: []string{"redis:7"},
						InitConfig:     &apiextensionsv1.JSON{Raw: []byte(`{}`)},
						Instances:      []apiextensionsv1.JSON{{Raw: []byte(`{"host":"%%host%%","port":"6379"}`)}},
					},
				},
			},
		},
	}

	entries, err := convertCR(dwc)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	assert.Contains(t, entries, "default_multi-check_http_check.yaml")
	assert.Contains(t, entries, "default_multi-check_redisdb.yaml")

	assert.Contains(t, entries["default_multi-check_http_check.yaml"], "myapp:v1")
	assert.Contains(t, entries["default_multi-check_redisdb.yaml"], "redis:7")
}

func TestConvertCR_NilInitConfig(t *testing.T) {
	dwc := &datadoghq.DatadogInstrumentation{
		ObjectMeta: metav1.ObjectMeta{Name: "check", Namespace: "default"},
		Spec: datadoghq.DatadogInstrumentationSpec{
			Selector: datadoghq.PodSelector{MatchLabels: map[string]string{"app": "x"}},
			Config: datadoghq.InstrumentationConfig{
				Checks: []datadoghq.CheckConfig{
					{
						Integration: "http_check",
						Instances:   []apiextensionsv1.JSON{{Raw: []byte(`{"url":"http://localhost"}`)}},
					},
				},
			},
		},
	}

	entries, err := convertCR(dwc)
	require.NoError(t, err)

	yaml := entries["default_check_http_check.yaml"]
	assert.Contains(t, yaml, "init_config: null")
}

func TestConvertCR_WithLogs(t *testing.T) {
	dwc := &datadoghq.DatadogInstrumentation{
		ObjectMeta: metav1.ObjectMeta{Name: "check", Namespace: "default"},
		Spec: datadoghq.DatadogInstrumentationSpec{
			Selector: datadoghq.PodSelector{MatchLabels: map[string]string{"app": "x"}},
			Config: datadoghq.InstrumentationConfig{
				Checks: []datadoghq.CheckConfig{
					{
						Integration: "http_check",
						Instances:   []apiextensionsv1.JSON{{Raw: []byte(`{"url":"http://localhost"}`)}},
						Logs:        &apiextensionsv1.JSON{Raw: []byte(`[{"type":"file","path":"/var/log/app.log","service":"myapp"}]`)},
					},
				},
			},
		},
	}

	entries, err := convertCR(dwc)
	require.NoError(t, err)

	yaml := entries["default_check_http_check.yaml"]
	assert.Contains(t, yaml, "logs:")
	assert.Contains(t, yaml, "service: myapp")
}

func TestConvertCR_WithAnnotationSelector(t *testing.T) {
	dwc := &datadoghq.DatadogInstrumentation{
		ObjectMeta: metav1.ObjectMeta{Name: "check", Namespace: "web-team"},
		Spec: datadoghq.DatadogInstrumentationSpec{
			Selector: datadoghq.PodSelector{
				MatchAnnotations: map[string]string{
					"team": "web",
					"env":  "prod",
				},
			},
			Config: datadoghq.InstrumentationConfig{
				Checks: []datadoghq.CheckConfig{
					{
						Integration: "nginx",
						Instances:   []apiextensionsv1.JSON{{Raw: []byte(`{"url":"http://%%host%%"}`)}},
					},
				},
			},
		},
	}

	entries, err := convertCR(dwc)
	require.NoError(t, err)

	yaml := entries["web-team_check_nginx.yaml"]
	assert.Contains(t, yaml, "cel_selector:")
	assert.Contains(t, yaml, `annotations["env"]`)
	assert.Contains(t, yaml, `annotations["team"]`)
	assert.Contains(t, yaml, `namespace == 'web-team'`)
}

func TestConvertCR_WithLabelSelector(t *testing.T) {
	dwc := &datadoghq.DatadogInstrumentation{
		ObjectMeta: metav1.ObjectMeta{Name: "check", Namespace: "default"},
		Spec: datadoghq.DatadogInstrumentationSpec{
			Selector: datadoghq.PodSelector{
				MatchLabels: map[string]string{"app": "nginx"},
			},
			Config: datadoghq.InstrumentationConfig{
				Checks: []datadoghq.CheckConfig{
					{
						Integration: "nginx",
						Instances:   []apiextensionsv1.JSON{{Raw: []byte(`{"url":"http://%%host%%"}`)}},
					},
				},
			},
		},
	}

	entries, err := convertCR(dwc)
	require.NoError(t, err)

	yaml := entries["default_check_nginx.yaml"]
	assert.Contains(t, yaml, "cel_selector:")
	assert.Contains(t, yaml, `container.pod.labels["app"] == "nginx"`)
	assert.Contains(t, yaml, `container.pod.namespace == 'default'`)
}

func TestConvertCR_WithBothLabelAndAnnotationSelector(t *testing.T) {
	dwc := &datadoghq.DatadogInstrumentation{
		ObjectMeta: metav1.ObjectMeta{Name: "check", Namespace: "web-team"},
		Spec: datadoghq.DatadogInstrumentationSpec{
			Selector: datadoghq.PodSelector{
				MatchLabels:      map[string]string{"app": "nginx"},
				MatchAnnotations: map[string]string{"team": "web"},
			},
			Config: datadoghq.InstrumentationConfig{
				Checks: []datadoghq.CheckConfig{
					{
						Integration: "nginx",
						Instances:   []apiextensionsv1.JSON{{Raw: []byte(`{"url":"http://%%host%%"}`)}},
					},
				},
			},
		},
	}

	entries, err := convertCR(dwc)
	require.NoError(t, err)

	yaml := entries["web-team_check_nginx.yaml"]
	assert.Contains(t, yaml, `container.pod.annotations["team"] == "web"`)
	assert.Contains(t, yaml, `container.pod.labels["app"] == "nginx"`)
	assert.Contains(t, yaml, `container.pod.namespace == 'web-team'`)
}

func TestConvertCR_NoSelector(t *testing.T) {
	dwc := &datadoghq.DatadogInstrumentation{
		ObjectMeta: metav1.ObjectMeta{Name: "check", Namespace: "default"},
		Spec: datadoghq.DatadogInstrumentationSpec{
			Selector: datadoghq.PodSelector{},
			Config: datadoghq.InstrumentationConfig{
				Checks: []datadoghq.CheckConfig{
					{
						Integration:    "nginx",
						ContainerImage: []string{"nginx"},
						Instances:      []apiextensionsv1.JSON{{Raw: []byte(`{"url":"http://%%host%%"}`)}},
					},
				},
			},
		},
	}

	entries, err := convertCR(dwc)
	require.NoError(t, err)

	yaml := entries["default_check_nginx.yaml"]
	assert.Contains(t, yaml, "cel_selector:")
	assert.Contains(t, yaml, `container.pod.namespace == 'default'`)
}

func TestConfigMapKey(t *testing.T) {
	assert.Equal(t, "web-team_nginx-check_nginx.yaml", configMapKey("web-team", "nginx-check", "nginx"))
}

func TestConvertCR_InvalidJSON(t *testing.T) {
	dwc := &datadoghq.DatadogInstrumentation{
		ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "default"},
		Spec: datadoghq.DatadogInstrumentationSpec{
			Selector: datadoghq.PodSelector{MatchLabels: map[string]string{"app": "x"}},
			Config: datadoghq.InstrumentationConfig{
				Checks: []datadoghq.CheckConfig{
					{
						Integration: "test",
						InitConfig:  &apiextensionsv1.JSON{Raw: []byte(`not-json`)},
						Instances:   []apiextensionsv1.JSON{{Raw: []byte(`{"ok": true}`)}},
					},
				},
			},
		},
	}

	_, err := convertCR(dwc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "initConfig")
}

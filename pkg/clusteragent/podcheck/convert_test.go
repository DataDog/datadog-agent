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

func TestConvertCR_SingleCheck(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-check",
			Namespace: "web-team",
		},
		Spec: datadoghq.DatadogPodCheckSpec{
			Selector: datadoghq.PodSelector{
				MatchLabels: map[string]string{"app": "nginx"},
			},
			Checks: []datadoghq.CheckConfig{
				{
					Name:          "nginx",
					ADIdentifiers: []string{"nginx:latest"},
					InitConfig:    &apiextensionsv1.JSON{Raw: []byte(`{}`)},
					Instances: []apiextensionsv1.JSON{
						{Raw: []byte(`{"nginx_status_url":"http://%%host%%:81/status/"}`)},
					},
				},
			},
		},
	}

	entries, err := convertCR(dpc)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	yaml := entries["web-team_nginx-check_nginx.yaml"]
	assert.Contains(t, yaml, "ad_identifiers:\n- nginx:latest")
	assert.Contains(t, yaml, "init_config: {}")
	assert.Contains(t, yaml, "nginx_status_url")
	assert.Contains(t, yaml, `container.pod.labels["app"] == "nginx"`)
}

func TestConvertCR_MultipleChecks(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-check",
			Namespace: "default",
		},
		Spec: datadoghq.DatadogPodCheckSpec{
			Selector: datadoghq.PodSelector{
				MatchLabels: map[string]string{"app": "myapp"},
			},
			Checks: []datadoghq.CheckConfig{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"myapp:v1"},
					Instances:     []apiextensionsv1.JSON{{Raw: []byte(`{"url":"http://%%host%%:8080"}`)}},
				},
				{
					Name:          "redisdb",
					ADIdentifiers: []string{"redis:7"},
					InitConfig:    &apiextensionsv1.JSON{Raw: []byte(`{}`)},
					Instances:     []apiextensionsv1.JSON{{Raw: []byte(`{"host":"%%host%%","port":"6379"}`)}},
				},
			},
		},
	}

	entries, err := convertCR(dpc)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	assert.Contains(t, entries, "default_multi-check_http_check.yaml")
	assert.Contains(t, entries, "default_multi-check_redisdb.yaml")

	assert.Contains(t, entries["default_multi-check_http_check.yaml"], "myapp:v1")
	assert.Contains(t, entries["default_multi-check_redisdb.yaml"], "redis:7")
}

func TestConvertCR_NilInitConfig(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "check", Namespace: "default"},
		Spec: datadoghq.DatadogPodCheckSpec{
			Selector: datadoghq.PodSelector{MatchLabels: map[string]string{"app": "x"}},
			Checks: []datadoghq.CheckConfig{
				{
					Name:      "http_check",
					Instances: []apiextensionsv1.JSON{{Raw: []byte(`{"url":"http://localhost"}`)}},
				},
			},
		},
	}

	entries, err := convertCR(dpc)
	require.NoError(t, err)

	yaml := entries["default_check_http_check.yaml"]
	assert.Contains(t, yaml, "init_config: null")
}

func TestConvertCR_WithLogs(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "check", Namespace: "default"},
		Spec: datadoghq.DatadogPodCheckSpec{
			Selector: datadoghq.PodSelector{MatchLabels: map[string]string{"app": "x"}},
			Checks: []datadoghq.CheckConfig{
				{
					Name:      "http_check",
					Instances: []apiextensionsv1.JSON{{Raw: []byte(`{"url":"http://localhost"}`)}},
					Logs:      &apiextensionsv1.JSON{Raw: []byte(`[{"type":"file","path":"/var/log/app.log","service":"myapp"}]`)},
				},
			},
		},
	}

	entries, err := convertCR(dpc)
	require.NoError(t, err)

	yaml := entries["default_check_http_check.yaml"]
	assert.Contains(t, yaml, "logs:")
	assert.Contains(t, yaml, "service: myapp")
}

func TestConvertCR_WithAnnotationSelector(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "check", Namespace: "web-team"},
		Spec: datadoghq.DatadogPodCheckSpec{
			Selector: datadoghq.PodSelector{
				MatchAnnotations: map[string]string{
					"team": "web",
					"env":  "prod",
				},
			},
			Checks: []datadoghq.CheckConfig{
				{
					Name:      "nginx",
					Instances: []apiextensionsv1.JSON{{Raw: []byte(`{"url":"http://%%host%%"}`)}},
				},
			},
		},
	}

	entries, err := convertCR(dpc)
	require.NoError(t, err)

	yaml := entries["web-team_check_nginx.yaml"]
	assert.Contains(t, yaml, "cel_selector:")
	// YAML may wrap long lines, so check for key fragments
	assert.Contains(t, yaml, `annotations["env"]`)
	assert.Contains(t, yaml, `annotations["team"]`)
	assert.Contains(t, yaml, `namespace == 'web-team'`)
}

func TestConvertCR_WithLabelSelector(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "check", Namespace: "default"},
		Spec: datadoghq.DatadogPodCheckSpec{
			Selector: datadoghq.PodSelector{
				MatchLabels: map[string]string{"app": "nginx"},
			},
			Checks: []datadoghq.CheckConfig{
				{
					Name:      "nginx",
					Instances: []apiextensionsv1.JSON{{Raw: []byte(`{"url":"http://%%host%%"}`)}},
				},
			},
		},
	}

	entries, err := convertCR(dpc)
	require.NoError(t, err)

	yaml := entries["default_check_nginx.yaml"]
	assert.Contains(t, yaml, "cel_selector:")
	assert.Contains(t, yaml, `container.pod.labels["app"] == "nginx"`)
	assert.Contains(t, yaml, `container.pod.namespace == 'default'`)
}

func TestConvertCR_WithBothLabelAndAnnotationSelector(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "check", Namespace: "web-team"},
		Spec: datadoghq.DatadogPodCheckSpec{
			Selector: datadoghq.PodSelector{
				MatchLabels:      map[string]string{"app": "nginx"},
				MatchAnnotations: map[string]string{"team": "web"},
			},
			Checks: []datadoghq.CheckConfig{
				{
					Name:      "nginx",
					Instances: []apiextensionsv1.JSON{{Raw: []byte(`{"url":"http://%%host%%"}`)}},
				},
			},
		},
	}

	entries, err := convertCR(dpc)
	require.NoError(t, err)

	yaml := entries["web-team_check_nginx.yaml"]
	assert.Contains(t, yaml, `container.pod.annotations["team"] == "web"`)
	assert.Contains(t, yaml, `container.pod.labels["app"] == "nginx"`)
	assert.Contains(t, yaml, `container.pod.namespace == 'web-team'`)
}

func TestConvertCR_NoSelector(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "check", Namespace: "default"},
		Spec: datadoghq.DatadogPodCheckSpec{
			Selector: datadoghq.PodSelector{},
			Checks: []datadoghq.CheckConfig{
				{
					Name:          "nginx",
					ADIdentifiers: []string{"nginx"},
					Instances:     []apiextensionsv1.JSON{{Raw: []byte(`{"url":"http://%%host%%"}`)}},
				},
			},
		},
	}

	entries, err := convertCR(dpc)
	require.NoError(t, err)

	yaml := entries["default_check_nginx.yaml"]
	assert.Contains(t, yaml, "cel_selector:")
	assert.Contains(t, yaml, `container.pod.namespace == 'default'`)
}

func TestConfigMapKey(t *testing.T) {
	assert.Equal(t, "web-team_nginx-check_nginx.yaml", configMapKey("web-team", "nginx-check", "nginx"))
}

func TestBuildMapCELRules(t *testing.T) {
	rules := buildMapCELRules("container.pod.annotations", map[string]string{
		"b-key": "val-b",
		"a-key": "val-a",
	})

	require.Len(t, rules, 2)
	assert.Equal(t, `container.pod.annotations["a-key"] == "val-a"`, rules[0])
	assert.Equal(t, `container.pod.annotations["b-key"] == "val-b"`, rules[1])
}

func TestBuildMapCELRules_Labels(t *testing.T) {
	rules := buildMapCELRules("container.pod.labels", map[string]string{"app": "nginx"})
	require.Len(t, rules, 1)
	assert.Equal(t, `container.pod.labels["app"] == "nginx"`, rules[0])
}

func TestBuildMapCELRules_Empty(t *testing.T) {
	assert.Nil(t, buildMapCELRules("container.pod.labels", nil))
}

func TestConvertCR_InvalidJSON(t *testing.T) {
	dpc := &datadoghq.DatadogPodCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "default"},
		Spec: datadoghq.DatadogPodCheckSpec{
			Selector: datadoghq.PodSelector{MatchLabels: map[string]string{"app": "x"}},
			Checks: []datadoghq.CheckConfig{
				{
					Name:       "test",
					InitConfig: &apiextensionsv1.JSON{Raw: []byte(`not-json`)},
					Instances:  []apiextensionsv1.JSON{{Raw: []byte(`{"ok": true}`)}},
				},
			},
		},
	}

	_, err := convertCR(dpc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "initConfig")
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func BenchmarkNoRegexMatching1(b *testing.B)        { benchmarkMatching(1, b) }
func BenchmarkNoRegexMatching10(b *testing.B)       { benchmarkMatching(10, b) }
func BenchmarkNoRegexMatching100(b *testing.B)      { benchmarkMatching(100, b) }
func BenchmarkNoRegexMatching1000(b *testing.B)     { benchmarkMatching(1000, b) }
func BenchmarkRegexMatchingCustom1000(b *testing.B) { benchmarkMatchingCustomRegex(1000, b) }

//goland:noinspection ALL
var avoidOptContainer v1.Container

func benchmarkMatching(nbContainers int, b *testing.B) {
	containersBenchmarks := make([]v1.Container, nbContainers)
	containersToBenchmark := make([]v1.Container, nbContainers)
	c := v1.Container{}

	scrubber := NewDefaultDataScrubber()
	for _, testCase := range getScrubCases() {
		containersToBenchmark = append(containersToBenchmark, testCase.input)
	}
	for i := 0; i < nbContainers; i++ {
		containersBenchmarks = append(containersBenchmarks, containersToBenchmark...)
	}
	b.ResetTimer()

	b.Run("simplified", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			for _, c := range containersBenchmarks {
				scrubContainer(&c, scrubber)
			}
		}
	})
	avoidOptContainer = c
}

func benchmarkMatchingCustomRegex(nbContainers int, b *testing.B) {
	var containersBenchmarks []v1.Container
	var containersToBenchmark []v1.Container
	c := v1.Container{}

	customRegs := []string{"pwd*", "*test"}
	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveRegex(customRegs)

	for _, testCase := range getScrubCases() {
		containersToBenchmark = append(containersToBenchmark, testCase.input)
	}
	for i := 0; i < nbContainers; i++ {
		containersBenchmarks = append(containersBenchmarks, containersToBenchmark...)
	}

	b.ResetTimer()
	b.Run("simplified", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			for _, c := range containersBenchmarks {
				scrubContainer(&c, scrubber)
			}
		}
	})

	avoidOptContainer = c
}

func TestRemoveSensitiveAnnotations(t *testing.T) {
	objectMeta := metav1.ObjectMeta{
		Annotations: map[string]string{
			v1.LastAppliedConfigAnnotation: `{"apiVersion":"v1","kind":"Pod","metadata":{"annotations":{},"name":"quota","namespace":"default"},"spec":{"containers":[{"args":["-c","while true; do echo hello; sleep 10;done"],"command":["/bin/sh"],"image":"ubuntu","name":"high-priority","resources":{"limits":{"cpu":"500m","memory":"10Gi"},"requests":{"cpu":"500m","memory":"10Gi"}}}],"priorityClassName":"high-priority"}}`,
			consulOriginalPodAnnotation:    `{"content": "previous pod definition"}`,
		},
		Labels: map[string]string{
			v1.LastAppliedConfigAnnotation: `{"apiVersion":"v1","kind":"Pod","metadata":{"annotations":{},"name":"quota","namespace":"default"},"spec":{"containers":[{"args":["-c","while true; do echo hello; sleep 10;done"],"command":["/bin/sh"],"image":"ubuntu","name":"high-priority","resources":{"limits":{"cpu":"500m","memory":"10Gi"},"requests":{"cpu":"500m","memory":"10Gi"}}}],"priorityClassName":"high-priority"}}`,
			consulOriginalPodAnnotation:    `{"content": "previous pod definition"}`,
			"other-labels":                 "value",
		},
	}
	RemoveSensitiveAnnotationsAndLabels(objectMeta.Annotations, objectMeta.Labels)
	expected := metav1.ObjectMeta{
		Annotations: map[string]string{
			v1.LastAppliedConfigAnnotation: redactedAnnotationValue,
			consulOriginalPodAnnotation:    redactedAnnotationValue,
		},
		Labels: map[string]string{
			v1.LastAppliedConfigAnnotation: redactedAnnotationValue,
			consulOriginalPodAnnotation:    redactedAnnotationValue,
			"other-labels":                 "value",
		},
	}
	assert.Equal(t, expected, objectMeta)
}

func TestRemoveSensitiveAnnotationsNotPresent(t *testing.T) {
	objectMeta := metav1.ObjectMeta{Annotations: map[string]string{
		"not.last.applied.annotation": "value",
	}}

	RemoveSensitiveAnnotationsAndLabels(objectMeta.Annotations, objectMeta.Labels)
	actual := objectMeta.Annotations[v1.LastAppliedConfigAnnotation]
	assert.Equal(t, "", actual)
}

func TestRemoveSensitiveAnnotationsEmpty(t *testing.T) {
	objectMeta := metav1.ObjectMeta{Annotations: map[string]string{
		v1.LastAppliedConfigAnnotation: "",
	}}

	RemoveSensitiveAnnotationsAndLabels(objectMeta.Annotations, objectMeta.Labels)
	actual := objectMeta.Annotations[v1.LastAppliedConfigAnnotation]
	assert.Equal(t, redactedAnnotationValue, actual)
}

func TestScrubPod(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"ad.datadoghq.com/postgres.logs":           `[{"source":"postgresql","service":"postgresql"}]`,
				"cni.projectcalico.org/podIPs":             `10.1.91.209/32`,
				"kubernetes.io/config.seen":                `2023-09-12T14:45:38.769079271Z`,
				"kubernetes.io/config.source":              `api`,
				"ad.datadoghq.com/http_check.instances":    `[{"url": "%%host%%", "username": "admin", "password": "test1234"}]`,
				"ad.datadoghq.com/http_check.init_configs": `[{}]`,
				"cni.projectcalico.org/containerID":        `3e1aac38cc95af5899db757c5fb7b7cddb96c6dfe979378b785a4e5e732954e8`,
				"cni.projectcalico.org/podIP":              `10.1.91.209/32`,
				"ad.datadoghq.com/postgres.check_names":    `["http_check"]`,
			},
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				{
					Name:    "init-1",
					Image:   "bootstrap",
					Command: []string{"bootstrap", "--crash-on-error", "start", "--password", "dKbgOrFmlhkyjGPkmuUtXwnOsxRXevnsef"},
					Args:    []string{},
					Env: []v1.EnvVar{
						{Name: "LOG_LEVEL", Value: "DEBUG"},
						{Name: "VAULT_ADDR", Value: "https://vault.domain.com"},
						{Name: "VAULT_AUTH_PATH", Value: "/domain/cluster"},
						{Name: "AVAILABILITY_ZONE", Value: "zone-1"},
					},
				},
			},
			Containers: []v1.Container{
				{
					Name:    "container-1",
					Image:   "app",
					Command: []string{"app", "--parameter", "value", "--password", "fOPeWWuFKUxwGLRTNnoM٪YwCExdwUcQBDZMogm"},
					Args:    []string{"--token", "GgItUUMOmܪnwPwcJNKbhwfutPmUgGXKHGin", "--other-parameter", "other-value"},
					Env: []v1.EnvVar{
						{Name: "API_KEY", Value: "LkhqmrnfESPrvhyfpephDDokCKvVokxXg"},
						{Name: "DD_SITE", Value: "datadoghq.com"},
					},
					LivenessProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								HTTPHeaders: []v1.HTTPHeader{
									{
										Name:  "Authorization",
										Value: "Bearer GgItUUMOmܪnwPwcJNKbhwfutPmUgGXKHGin",
									},
								},
							},
							Exec: &v1.ExecAction{
								Command: []string{"/hello", "--password", "fOPeWWuFKUxwGLRTNnoM٪YwCExdwUcQBDZMogm"},
							},
						},
					},
					ReadinessProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								HTTPHeaders: []v1.HTTPHeader{
									{
										Name:  "Authorization",
										Value: "Bearer GgItUUMOmܪnwPwcJNKbhwfutPmUgGXKHGin",
									},
								},
							},
						},
					},
					StartupProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								HTTPHeaders: []v1.HTTPHeader{
									{
										Name:  "Authorization",
										Value: "Bearer GgItUUMOmܪnwPwcJNKbhwfutPmUgGXKHGin",
									},
								},
							},
						},
					},
				},
				{
					Name:    "container-2",
					Image:   "sidecard",
					Command: []string{"sidecard", "--password", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9", "--verbose"},
					Args:    []string{"--timeout", "10m", "--credentials", "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"},
					Env:     []v1.EnvVar{},
				},
			},
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	expectedPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"ad.datadoghq.com/postgres.logs":           `[{"source":"postgresql","service":"postgresql"}]`,
				"cni.projectcalico.org/podIPs":             `10.1.91.209/32`,
				"kubernetes.io/config.seen":                `2023-09-12T14:45:38.769079271Z`,
				"kubernetes.io/config.source":              `api`,
				"ad.datadoghq.com/http_check.instances":    `[{"url": "%%host%%", "username": "********", "password": "********"}]`,
				"ad.datadoghq.com/http_check.init_configs": `[{}]`,
				"cni.projectcalico.org/containerID":        `3e1aac38cc95af5899db757c5fb7b7cddb96c6dfe979378b785a4e5e732954e8`,
				"cni.projectcalico.org/podIP":              `10.1.91.209/32`,
				"ad.datadoghq.com/postgres.check_names":    `["http_check"]`,
			},
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				{
					Name:    "init-1",
					Image:   "bootstrap",
					Command: []string{"bootstrap", "--crash-on-error", "start", "--password", "********"},
					Args:    []string{},
					Env: []v1.EnvVar{
						{Name: "LOG_LEVEL", Value: "DEBUG"},
						{Name: "VAULT_ADDR", Value: "********"},
						{Name: "VAULT_AUTH_PATH", Value: "********"},
						{Name: "AVAILABILITY_ZONE", Value: "zone-1"},
					},
				},
			},
			Containers: []v1.Container{
				{
					Name:    "container-1",
					Image:   "app",
					Command: []string{"app", "--parameter", "value", "--password", "********"},
					Args:    []string{"--token", "********", "--other-parameter", "other-value"},
					Env: []v1.EnvVar{
						{Name: "API_KEY", Value: "********"},
						{Name: "DD_SITE", Value: "datadoghq.com"},
					},
					LivenessProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								HTTPHeaders: []v1.HTTPHeader{
									{
										Name:  "Authorization",
										Value: "********",
									},
								},
							},
							Exec: &v1.ExecAction{
								Command: []string{"/hello", "--password", "********"},
							},
						},
					},
					ReadinessProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								HTTPHeaders: []v1.HTTPHeader{
									{
										Name:  "Authorization",
										Value: "********",
									},
								},
							},
						},
					},
					StartupProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								HTTPHeaders: []v1.HTTPHeader{
									{
										Name:  "Authorization",
										Value: "********",
									},
								},
							},
						},
					},
				},
				{
					Name:    "container-2",
					Image:   "sidecard",
					Command: []string{"sidecard", "--password", "********", "--verbose"},
					Args:    []string{"--timeout", "10m", "--credentials", "********"},
					Env:     []v1.EnvVar{},
				},
			},
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords([]string{"token", "username", "vault", "authorization"})
	ScrubPod(pod, scrubber)

	assert.EqualValues(t, expectedPod, pod)
}

func TestScrubPodTemplate(t *testing.T) {
	template := &v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"ad.datadoghq.com/postgres.logs":           `[{"source":"postgresql","service":"postgresql"}]`,
				"cni.projectcalico.org/podIPs":             `10.1.91.209/32`,
				"kubernetes.io/config.seen":                `2023-09-12T14:45:38.769079271Z`,
				"kubernetes.io/config.source":              `api`,
				"ad.datadoghq.com/http_check.instances":    `[{"url": "%%host%%", "username": "admin", "password": "test1234"}]`,
				"ad.datadoghq.com/http_check.init_configs": `[{}]`,
				"cni.projectcalico.org/containerID":        `3e1aac38cc95af5899db757c5fb7b7cddb96c6dfe979378b785a4e5e732954e8`,
				"cni.projectcalico.org/podIP":              `10.1.91.209/32`,
				"ad.datadoghq.com/postgres.check_names":    `["http_check"]`,
			},
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				{
					Name:    "init-1",
					Image:   "bootstrap",
					Command: []string{"bootstrap", "--crash-on-error", "start", "--password", "dKbgOrFmlhkyjGPkmuUtXwnOsxRXevnsef"},
					Args:    []string{},
					Env: []v1.EnvVar{
						{Name: "LOG_LEVEL", Value: "DEBUG"},
						{Name: "VAULT_ADDR", Value: "https://vault.domain.com"},
						{Name: "VAULT_AUTH_PATH", Value: "/domain/cluster"},
						{Name: "AVAILABILITY_ZONE", Value: "zone-1"},
					},
				},
			},
			Containers: []v1.Container{
				{
					Name:    "container-1",
					Image:   "app",
					Command: []string{"app", "--parameter", "value", "--password", "fOPeWWuFKUxwGLRTNnoM٪YwCExdwUcQBDZMogm"},
					Args:    []string{"--token", "GgItUUMOmܪnwPwcJNKbhwfutPmUgGXKHGin", "--other-parameter", "other-value"},
					Env: []v1.EnvVar{
						{Name: "API_KEY", Value: "LkhqmrnfESPrvhyfpephDDokCKvVokxXg"},
						{Name: "DD_SITE", Value: "datadoghq.com"},
					},
					LivenessProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								HTTPHeaders: []v1.HTTPHeader{
									{
										Name:  "Authorization",
										Value: "Bearer GgItUUMOmܪnwPwcJNKbhwfutPmUgGXKHGin",
									},
								},
							},
							Exec: &v1.ExecAction{
								Command: []string{"/hello", "--password", "fOPeWWuFKUxwGLRTNnoM٪YwCExdwUcQBDZMogm"},
							},
						},
					},
					ReadinessProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								HTTPHeaders: []v1.HTTPHeader{
									{
										Name:  "Authorization",
										Value: "Bearer GgItUUMOmܪnwPwcJNKbhwfutPmUgGXKHGin",
									},
								},
							},
						},
					},
					StartupProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								HTTPHeaders: []v1.HTTPHeader{
									{
										Name:  "Authorization",
										Value: "Bearer GgItUUMOmܪnwPwcJNKbhwfutPmUgGXKHGin",
									},
								},
							},
						},
					},
				},
				{
					Name:    "container-2",
					Image:   "sidecard",
					Command: []string{"sidecard", "--password", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9", "--verbose"},
					Args:    []string{"--timeout", "10m", "--credentials", "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"},
					Env:     []v1.EnvVar{},
				},
			},
		},
	}

	expectedTemplate := &v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"ad.datadoghq.com/postgres.logs":           `[{"source":"postgresql","service":"postgresql"}]`,
				"cni.projectcalico.org/podIPs":             `10.1.91.209/32`,
				"kubernetes.io/config.seen":                `2023-09-12T14:45:38.769079271Z`,
				"kubernetes.io/config.source":              `api`,
				"ad.datadoghq.com/http_check.instances":    `[{"url": "%%host%%", "username": "********", "password": "********"}]`,
				"ad.datadoghq.com/http_check.init_configs": `[{}]`,
				"cni.projectcalico.org/containerID":        `3e1aac38cc95af5899db757c5fb7b7cddb96c6dfe979378b785a4e5e732954e8`,
				"cni.projectcalico.org/podIP":              `10.1.91.209/32`,
				"ad.datadoghq.com/postgres.check_names":    `["http_check"]`,
			},
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				{
					Name:    "init-1",
					Image:   "bootstrap",
					Command: []string{"bootstrap", "--crash-on-error", "start", "--password", "********"},
					Args:    []string{},
					Env: []v1.EnvVar{
						{Name: "LOG_LEVEL", Value: "DEBUG"},
						{Name: "VAULT_ADDR", Value: "********"},
						{Name: "VAULT_AUTH_PATH", Value: "********"},
						{Name: "AVAILABILITY_ZONE", Value: "zone-1"},
					},
				},
			},
			Containers: []v1.Container{
				{
					Name:    "container-1",
					Image:   "app",
					Command: []string{"app", "--parameter", "value", "--password", "********"},
					Args:    []string{"--token", "********", "--other-parameter", "other-value"},
					Env: []v1.EnvVar{
						{Name: "API_KEY", Value: "********"},
						{Name: "DD_SITE", Value: "datadoghq.com"},
					},
					LivenessProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								HTTPHeaders: []v1.HTTPHeader{
									{
										Name:  "Authorization",
										Value: "********",
									},
								},
							},
							Exec: &v1.ExecAction{
								Command: []string{"/hello", "--password", "********"},
							},
						},
					},
					ReadinessProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								HTTPHeaders: []v1.HTTPHeader{
									{
										Name:  "Authorization",
										Value: "********",
									},
								},
							},
						},
					},
					StartupProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								HTTPHeaders: []v1.HTTPHeader{
									{
										Name:  "Authorization",
										Value: "********",
									},
								},
							},
						},
					},
				},
				{
					Name:    "container-2",
					Image:   "sidecard",
					Command: []string{"sidecard", "--password", "********", "--verbose"},
					Args:    []string{"--timeout", "10m", "--credentials", "********"},
					Env:     []v1.EnvVar{},
				},
			},
		},
	}

	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords([]string{"token", "username", "vault", "authorization"})
	ScrubPodTemplateSpec(template, scrubber)

	assert.EqualValues(t, expectedTemplate, template)
}

func TestScrubAnnotations(t *testing.T) {
	annotations := map[string]string{
		"ad.datadoghq.com/postgres.logs":         `[{"source":"postgresql","service":"postgresql"}]`,
		"cni.projectcalico.org/podIPs":           `10.1.91.209/32`,
		"kubernetes.io/config.seen":              `2023-09-12T14:45:38.769079271Z`,
		"kubernetes.io/config.source":            `api`,
		"ad.datadoghq.com/postgres.instances":    `[{"host": "%%host%%", "port" : 5432, "username": "postgresadmin", "password": "test1234"}]`,
		"ad.datadoghq.com/postgres.init_configs": `[{}]`,
		"cni.projectcalico.org/containerID":      `3e1aac38cc95af5899db757c5fb7b7cddb96c6dfe979378b785a4e5e732954e8`,
		"cni.projectcalico.org/podIP":            `10.1.91.209/32`,
		"ad.datadoghq.com/postgres.check_names":  `["postgres"]`,
	}
	expectedAnnotations := map[string]string{
		"ad.datadoghq.com/postgres.logs":         `[{"source":"postgresql","service":"postgresql"}]`,
		"cni.projectcalico.org/podIPs":           `10.1.91.209/32`,
		"kubernetes.io/config.seen":              `2023-09-12T14:45:38.769079271Z`,
		"kubernetes.io/config.source":            `api`,
		"ad.datadoghq.com/postgres.instances":    `[{"host": "%%host%%", "port" : 5432, "username": "postgresadmin", "password": "********"}]`,
		"ad.datadoghq.com/postgres.init_configs": `[{}]`,
		"cni.projectcalico.org/containerID":      `3e1aac38cc95af5899db757c5fb7b7cddb96c6dfe979378b785a4e5e732954e8`,
		"cni.projectcalico.org/podIP":            `10.1.91.209/32`,
		"ad.datadoghq.com/postgres.check_names":  `["postgres"]`,
	}
	scrubber := NewDefaultDataScrubber()
	scrubAnnotations(annotations, scrubber)
	assert.Equal(t, expectedAnnotations, annotations)
}

func TestScrubContainer(t *testing.T) {
	scrubber := NewDefaultDataScrubber()
	tests := getScrubCases()
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scrubContainer(&tc.input, scrubber)
			assert.Equal(t, tc.expected, tc.input)
		})
	}
}

func getScrubCases() map[string]struct {
	input    v1.Container
	expected v1.Container
} {
	tests := map[string]struct {
		input    v1.Container
		expected v1.Container
	}{
		"sensitive CLI": {
			input: v1.Container{
				Command: []string{"mysql", "--password", "afztyerbzio1234"},
			},
			expected: v1.Container{
				Command: []string{"mysql", "--password", "********"},
			},
		},
		"empty container": {
			input:    v1.Container{},
			expected: v1.Container{},
		},
		"empty container empty slices": {
			input: v1.Container{
				Command: []string{},
				Args:    []string{},
			},
			expected: v1.Container{
				Command: []string{},
				Args:    []string{},
			},
		},
		"non sensitive CLI": {
			input: v1.Container{
				Command: []string{"mysql", "--arg", "afztyerbzio1234"},
			},
			expected: v1.Container{
				Command: []string{"mysql", "--arg", "afztyerbzio1234"},
			},
		},
		"non sensitive CLI joined": {
			input: v1.Container{
				Command: []string{"mysql --arg afztyerbzio1234"},
			},
			expected: v1.Container{
				Command: []string{"mysql --arg afztyerbzio1234"},
			},
		},
		"sensitive CLI joined": {
			input: v1.Container{
				Command: []string{"mysql --password afztyerbzio1234"},
			},
			expected: v1.Container{
				Command: []string{"mysql", "--password", "********"},
			},
		},
		"sensitive env var": {
			input: v1.Container{
				Env: []v1.EnvVar{{Name: "password", Value: "kqhkiG9w0BAQEFAASCAl8wggJbAgEAAoGBAOLJ"}},
			},
			expected: v1.Container{
				Env: []v1.EnvVar{{Name: "password", Value: "********"}},
			},
		},
		"sensitive env var set via ValueFrom": {
			input: v1.Container{
				Env: []v1.EnvVar{{
					Name: "password",
					ValueFrom: &v1.EnvVarSource{
						SecretKeyRef: &v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{Name: "my-secret"},
							Key:                  "password",
						},
					},
				}},
			},
			expected: v1.Container{
				Env: []v1.EnvVar{{
					Name: "password",
					ValueFrom: &v1.EnvVarSource{
						SecretKeyRef: &v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{Name: "my-secret"},
							Key:                  "password",
						},
					},
				}},
			},
		},
		"command with sensitive arg": {
			input: v1.Container{
				Command: []string{"mysql"},
				Args:    []string{"--password", "afztyerbzio1234"},
			},
			expected: v1.Container{
				Command: []string{"mysql"},
				Args:    []string{"--password", "********"},
			},
		},
		"command with no sensitive arg": {
			input: v1.Container{
				Command: []string{"mysql"},
				Args:    []string{"--debug", "afztyerbzio1234"},
			},
			expected: v1.Container{
				Command: []string{"mysql"},
				Args:    []string{"--debug", "afztyerbzio1234"},
			},
		},
		"sensitive command with no sensitive arg": {
			input: v1.Container{
				Command: []string{"mysql --password 123"},
				Args:    []string{"--debug", "123"},
			},
			expected: v1.Container{
				Command: []string{"mysql", "--password", "********"},
				Args:    []string{"--debug", "123"},
			},
		},
		"sensitive command with no sensitive arg joined": {
			input: v1.Container{
				Command: []string{"mysql --password 123"},
				Args:    []string{"--debug 123"},
			},
			expected: v1.Container{
				Command: []string{"mysql", "--password", "********"},
				Args:    []string{"--debug", "123"},
			},
		},
		"sensitive command with no sensitive arg split": {
			input: v1.Container{
				Command: []string{"mysql", "--password", "123"},
				Args:    []string{"--debug", "123"},
			},
			expected: v1.Container{
				Command: []string{"mysql", "--password", "********"},
				Args:    []string{"--debug", "123"},
			},
		},
		"sensitive command with no sensitive arg mixed": {
			input: v1.Container{
				Command: []string{"mysql", "--password", "123"},
				Args:    []string{"--debug", "123"},
			},
			expected: v1.Container{
				Command: []string{"mysql", "--password", "********"},
				Args:    []string{"--debug", "123"},
			},
		},
		"sensitive pass in args": {
			input: v1.Container{
				Command: []string{"agent --password"},
				Args:    []string{"token123"},
			},
			expected: v1.Container{
				Command: []string{"agent", "--password"},
				Args:    []string{"********"},
			},
		},
		"sensitive arg no command": {
			input: v1.Container{
				Args: []string{"password", "--password", "afztyerbzio1234"},
			},
			expected: v1.Container{
				Args: []string{"password", "--password", "********"},
			},
		},
		"sensitive arg joined": {
			input: v1.Container{
				Args: []string{"pwd pwd afztyerbzio1234 --password 1234"},
			},
			expected: v1.Container{
				Args: []string{"pwd", "pwd", "********", "--password", "********"},
			},
		},
		"sensitive container": {
			input: v1.Container{
				Name:    "test container",
				Image:   "random",
				Command: []string{"decrypt", "--password", "afztyerbzio1234", "--access_token", "yolo123"},
				Env: []v1.EnvVar{
					{Name: "hostname", Value: "password"},
					{Name: "pwd", Value: "yolo"},
				},
				Args: []string{"--password", "afztyerbzio1234"},
			},
			expected: v1.Container{
				Name:    "test container",
				Image:   "random",
				Command: []string{"decrypt", "--password", "********", "--access_token", "********"},
				Env: []v1.EnvVar{
					{Name: "hostname", Value: "password"},
					{Name: "pwd", Value: "********"},
				},
				Args: []string{"--password", "********"},
			},
		},
		"large script with bounds check": {
			input: v1.Container{
				Command: []string{"/bin/bash", "-c", `
if [ "$CLOUDPROVIDER" == "google" ]; then
  export SPARK_HISTORY_OPTS="$SPARK_HISTORY_OPTS \
  -Dspark.hadoop.fs.s3a.endpoint=s3-fips.amazonaws.com";


  export SPARK_DAEMON_JAVA_OPTS="$SPARK_DAEMON_JAVA_OPTS \
    -Djava.security.properties=/usr/local/etc/fips/java.security \
    -Djavax.net.ssl.trustStorePassword=123 \
    -Dorg.bouncycastle.fips.approved_only=true";
fi;
if [ "$CLOUDPROVIDER" == "azure" ]; then
  source /etc/datadog/azureSASEnv;

  export SPARK_HISTORY_OPTS="$SPARK_HISTORY_OPTS \
    -Dspark.hadoop.fs.azure.local.sas.key.mode=true;
fi;
/opt/spark/bin/spark-class org.apache.spark.deploy.history.HistoryServer;
	`},
				Args: []string{"--verbose"},
			},
			expected: v1.Container{
				Command: []string{"/bin/bash", "-c", "if", "[", "$CLOUDPROVIDER\" == \"", "google\" ]; then\n  export SPARK_HISTORY_OPTS=\"", "$SPARK_HISTORY_OPTS", "\\", "-Dspark.hadoop.fs.s3a.endpoint=s3-fips.amazonaws.com\";\n\n\n  export SPARK_DAEMON_JAVA_OPTS=\"", "$SPARK_DAEMON_JAVA_OPTS", "\\", "-Djava.security.properties=/usr/local/etc/fips/java.security", "\\", "-Djavax.net.ssl.trustStorePassword=********", "\\", "-Dorg.bouncycastle.fips.approved_only=true\";\nfi;\nif [ \"", "$CLOUDPROVIDER\" == \"", "azure\" ]; then\n  source /etc/datadog/azureSASEnv;\n\n  export SPARK_HISTORY_OPTS=\"", "$SPARK_HISTORY_OPTS", "\\", "-Dspark.hadoop.fs.azure.local.sas.key.mode=true;", "fi;", "/opt/spark/bin/spark-class", "org.apache.spark.deploy.history.HistoryServer;"},
				Args:    []string{"--verbose"},
			},
		},
		"large script with sensitive args": {
			input: v1.Container{
				Command: []string{"/bin/bash", "-c", `
if [ "$CLOUDPROVIDER" == "google" ]; then
  export SPARK_HISTORY_OPTS="$SPARK_HISTORY_OPTS \
  -Dspark.hadoop.fs.s3a.endpoint=s3-fips.amazonaws.com";


  export SPARK_DAEMON_JAVA_OPTS="$SPARK_DAEMON_JAVA_OPTS \
    -Djava.security.properties=/usr/local/etc/fips/java.security \
    -Djavax.net.ssl.trustStorePassword=123 \
    -Dorg.bouncycastle.fips.approved_only=true";
fi;
if [ "$CLOUDPROVIDER" == "azure" ]; then
  source /etc/datadog/azureSASEnv;

  export SPARK_HISTORY_OPTS="$SPARK_HISTORY_OPTS \
    -Dspark.hadoop.fs.azure.local.sas.key.mode=true;
fi;
/opt/spark/bin/spark-class org.apache.spark.deploy.history.HistoryServer; --password
	`},
				Args: []string{"123456", "--access_token", "123456"},
			},
			expected: v1.Container{
				Command: []string{"/bin/bash", "-c", "if", "[", "$CLOUDPROVIDER\" == \"", "google\" ]; then\n  export SPARK_HISTORY_OPTS=\"", "$SPARK_HISTORY_OPTS", "\\", "-Dspark.hadoop.fs.s3a.endpoint=s3-fips.amazonaws.com\";\n\n\n  export SPARK_DAEMON_JAVA_OPTS=\"", "$SPARK_DAEMON_JAVA_OPTS", "\\", "-Djava.security.properties=/usr/local/etc/fips/java.security", "\\", "-Djavax.net.ssl.trustStorePassword=********", "\\", "-Dorg.bouncycastle.fips.approved_only=true\";\nfi;\nif [ \"", "$CLOUDPROVIDER\" == \"", "azure\" ]; then\n  source /etc/datadog/azureSASEnv;\n\n  export SPARK_HISTORY_OPTS=\"", "$SPARK_HISTORY_OPTS", "\\", "-Dspark.hadoop.fs.azure.local.sas.key.mode=true;", "fi;", "/opt/spark/bin/spark-class", "org.apache.spark.deploy.history.HistoryServer;", "--password"},
				Args:    []string{"********", "--access_token", "********"},
			},
		},
	}
	return tests
}

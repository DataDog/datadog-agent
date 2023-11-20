// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRemoveLastAppliedConfigurationAnnotation(t *testing.T) {
	objectMeta := metav1.ObjectMeta{Annotations: map[string]string{
		v1.LastAppliedConfigAnnotation: `{"apiVersion":"v1","kind":"Pod","metadata":{"annotations":{},"name":"quota","namespace":"default"},"spec":{"containers":[{"args":["-c","while true; do echo hello; sleep 10;done"],"command":["/bin/sh"],"image":"ubuntu","name":"high-priority","resources":{"limits":{"cpu":"500m","memory":"10Gi"},"requests":{"cpu":"500m","memory":"10Gi"}}}],"priorityClassName":"high-priority"}}`,
	}}
	RemoveLastAppliedConfigurationAnnotation(objectMeta.Annotations)
	actual := objectMeta.Annotations[v1.LastAppliedConfigAnnotation]
	assert.Equal(t, redactedAnnotationValue, actual)
}

func TestRemoveLastAppliedConfigurationAnnotationNotPresent(t *testing.T) {
	objectMeta := metav1.ObjectMeta{Annotations: map[string]string{
		"not.last.applied.annotation": "value",
	}}

	RemoveLastAppliedConfigurationAnnotation(objectMeta.Annotations)
	actual := objectMeta.Annotations[v1.LastAppliedConfigAnnotation]
	assert.Equal(t, "", actual)
}

func TestRemoveLastAppliedConfigurationAnnotationEmpty(t *testing.T) {
	objectMeta := metav1.ObjectMeta{Annotations: map[string]string{
		v1.LastAppliedConfigAnnotation: "",
	}}

	RemoveLastAppliedConfigurationAnnotation(objectMeta.Annotations)
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
	scrubber.AddCustomSensitiveWords([]string{"token", "username", "vault"})
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
	scrubber.AddCustomSensitiveWords([]string{"token", "username", "vault"})
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
	}
	return tests
}

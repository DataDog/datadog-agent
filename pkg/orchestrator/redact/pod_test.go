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

func TestRedactAnnotations(t *testing.T) {
	lastAppliedConfiguration := `{
  "apiVersion": "apps/v1",
  "kind": "ReplicaSet",
  "metadata": {
    "annotations": {},
    "name": "gitlab-password-all",
    "namespace": "datadog-agent"
  },
  "spec": {
    "replicas": 1,
    "selector": {
      "matchLabels": {
        "tier": "frontend"
      }
    },
    "template": {
      "metadata": {
        "labels": {
          "tier": "frontend"
        }
      },
      "spec": {
        "containers": [
          {
            "env": [
              {
                "name": "GITLAB_TOKEN",
                "value": "test"
              },
              {
                "name": "TOKEN",
                "value": "test"
              },
              {
                "name": "password",
                "value": "test"
              },
              {
                "name": "secret",
                "value": "test"
              },
              {
                "name": "pwd",
                "value": "test"
              },
              {
                "name": "api_key",
                "value": "test"
              }
            ],
            "image": "gcr.io/google_samples/gb-frontend:v3",
            "name": "php-redis"
          }
        ]
      }
    }
  }
}`
	objectMeta := metav1.ObjectMeta{Annotations: map[string]string{
		"kubectl.kubernetes.io/last-applied-configuration": lastAppliedConfiguration,
	}}
	RemoveLastAppliedConfigurationAnnotation(objectMeta.Annotations)
	actual := objectMeta.Annotations["kubectl.kubernetes.io/last-applied-configuration"]
	expected := replacedValue
	assert.Equal(t, expected, actual)

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
	ScrubAnnotations(annotations, scrubber)
	assert.Equal(t, expectedAnnotations, annotations)
}

func TestRedactAnnotationsValueDoesNotExist(t *testing.T) {
	objectMeta := metav1.ObjectMeta{Annotations: map[string]string{
		"something/else": "some pseudo yaml",
	}}

	RemoveLastAppliedConfigurationAnnotation(objectMeta.Annotations)
	actual := objectMeta.Annotations["kubectl.kubernetes.io/last-applied-configuration"]
	expected := ""
	assert.Equal(t, expected, actual)
}

func TestRedactAnnotationsValueIsEmpty(t *testing.T) {
	objectMeta := metav1.ObjectMeta{Annotations: map[string]string{
		"kubectl.kubernetes.io/last-applied-configuration": "",
	}}

	RemoveLastAppliedConfigurationAnnotation(objectMeta.Annotations)
	actual := objectMeta.Annotations["kubectl.kubernetes.io/last-applied-configuration"]
	expected := replacedValue
	assert.Equal(t, expected, actual)
}

func TestScrubContainer(t *testing.T) {
	scrubber := NewDefaultDataScrubber()
	tests := getScrubCases()
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ScrubContainer(&tc.input, scrubber)
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

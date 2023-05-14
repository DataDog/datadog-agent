/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// getTestPod generates a new instance of an empty test Pod
func getTestPod(annotations map[string]string, podPriority *int32) *v1.Pod {
	pod := v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
	}
	// Set pod Priority in Spec if exists
	if podPriority != nil {
		pod.Spec = v1.PodSpec{
			Priority: podPriority,
		}
	}
	// Set annotations if exists
	if annotations != nil {
		pod.Annotations = annotations
	}
	return &pod
}

func configSourceAnnotation(source string) map[string]string {
	return map[string]string{ConfigSourceAnnotationKey: source}
}

func TestGetPodSource(t *testing.T) {
	tests := []struct {
		name        string
		pod         *v1.Pod
		expected    string
		errExpected bool
	}{
		{
			name:        "cannot get pod source",
			pod:         getTestPod(nil, nil),
			expected:    "",
			errExpected: true,
		},
		{
			name:        "valid annotation returns the source",
			pod:         getTestPod(configSourceAnnotation("host-ipc-sources"), nil),
			expected:    "host-ipc-sources",
			errExpected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, err := GetPodSource(test.pod)
			if test.errExpected {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.expected, source)
		})
	}
}

func TestIsStaticPod(t *testing.T) {
	tests := []struct {
		name     string
		pod      *v1.Pod
		expected bool
	}{
		{
			name:     "static pod with file source",
			pod:      getTestPod(configSourceAnnotation(FileSource), nil),
			expected: true,
		},
		{
			name:     "static pod with http source",
			pod:      getTestPod(configSourceAnnotation(HTTPSource), nil),
			expected: true,
		},
		{
			name:     "static pod with api server source",
			pod:      getTestPod(configSourceAnnotation(ApiserverSource), nil),
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			isStaticPod := IsStaticPod(test.pod)
			assert.Equal(t, test.expected, isStaticPod)
		})
	}
}

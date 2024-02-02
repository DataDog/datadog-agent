//go:build kubeapiserver

package agentsidecar

import (
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestInjectAgentSidecar(t *testing.T) {
	tests := []struct {
		Name         string
		Pod          *corev1.Pod
		ExpectError  bool
		ShouldInject bool
	}{
		{
			Name:         "should return error for nil pod",
			Pod:          nil,
			ExpectError:  true,
			ShouldInject: false,
		},
		{
			Name: "should inject sidecar if no sidecar present",
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-name",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "containe-name"},
					},
				},
			},
			ExpectError:  false,
			ShouldInject: true,
		},
		{
			Name: "should skip injecting sidecar when sidecar already exists",
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-name",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "containe-name"},
						{Name: agentSidecarContainerName},
					},
				},
			},
			ExpectError:  false,
			ShouldInject: false,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(tt *testing.T) {
			containersCount := 0

			if test.Pod != nil {
				containersCount = len(test.Pod.Spec.Containers)
			}

			err := injectAgentSidecar(test.Pod, "", nil)

			if test.ExpectError {
				assert.Error(tt, err, "expected non-nil error to be returned")
			} else {
				assert.NoError(tt, err, "expected returned error to be nil")

				if test.ShouldInject {
					assert.Equalf(tt, len(test.Pod.Spec.Containers), containersCount+1, "should inject sidecar")
				} else {
					assert.Equalf(tt, len(test.Pod.Spec.Containers), containersCount, "should not inject sidecar")
				}
			}

		})
	}

}

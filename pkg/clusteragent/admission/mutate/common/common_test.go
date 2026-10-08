// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"reflect"
	"testing"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
)

func TestMutate(t *testing.T) {
	tests := []struct {
		name    string
		rawPod  string
		wantPod string
		mutate  bool
	}{
		{
			name:    "unknown Pod spec field",
			rawPod:  `{"metadata":{"name":"test"},"spec":{"customField":{"enabled":true,"values":[1,null,"custom"]},"containers":[{"name":"app"}]}}`,
			wantPod: `{"metadata":{"name":"test","annotations":{"injected":"true"}},"spec":{"customField":{"enabled":true,"values":[1,null,"custom"]},"containers":[{"name":"app","env":[{"name":"INJECTED_ENV","value":"true"}]}]}}`,
			mutate:  true,
		},
		{
			name:    "unknown fields on multiple named containers",
			rawPod:  `{"metadata":{"name":"test"},"spec":{"containers":[{"name":"app","customField":42},{"name":"worker","customField":"worker-value"}]}}`,
			wantPod: `{"metadata":{"name":"test","annotations":{"injected":"true"}},"spec":{"containers":[{"name":"app","customField":42,"env":[{"name":"INJECTED_ENV","value":"true"}]},{"name":"worker","customField":"worker-value","env":[{"name":"INJECTED_ENV","value":"true"}]}]}}`,
			mutate:  true,
		},
		{
			name: "unknown nested field alongside known fields",
			rawPod: `{"metadata":{"name":"test"},"spec":{"containers":[{"name":"app",
				"securityContext":{"runAsNonRoot":true,"capabilities":{"add":["NET_ADMIN"],"drop":["ALL"],"customField":["custom-value"]}}}]}}`,
			wantPod: `{"metadata":{"name":"test","annotations":{"injected":"true"}},"spec":{"containers":[{"name":"app",
				"securityContext":{"runAsNonRoot":true,"capabilities":{"add":["NET_ADMIN"],"drop":["ALL"],"customField":["custom-value"]}},
				"env":[{"name":"INJECTED_ENV","value":"true"}]}]}}`,
			mutate: true,
		},
		{
			name: "unknown field only in nested object",
			rawPod: `{"metadata":{"name":"test"},"spec":{"containers":[{"name":"app",
				"securityContext":{"capabilities":{"customField":{"enabled":true,"values":[1,null,"custom"]}}}}]}}`,
			wantPod: `{"metadata":{"name":"test","annotations":{"injected":"true"}},"spec":{"containers":[{"name":"app",
				"securityContext":{"capabilities":{"customField":{"enabled":true,"values":[1,null,"custom"]}}},
				"env":[{"name":"INJECTED_ENV","value":"true"}]}]}}`,
			mutate: true,
		},
		{
			name:    "optional parent objects absent",
			rawPod:  `{"metadata":{"name":"test"},"spec":{"containers":[{"name":"app"}]}}`,
			wantPod: `{"metadata":{"name":"test","annotations":{"injected":"true"}},"spec":{"containers":[{"name":"app","env":[{"name":"INJECTED_ENV","value":"true"}]}]}}`,
			mutate:  true,
		},
		{
			name:    "no mutation preserves unknown fields",
			rawPod:  `{"metadata":{"name":"test"},"spec":{"customField":null,"containers":[{"name":"app","customField":false}]}}`,
			wantPod: `{"metadata":{"name":"test"},"spec":{"customField":null,"containers":[{"name":"app","customField":false}]}}`,
		},
		{
			name: "normalization without mutation",
			rawPod: `{"metadata":{"name":"test"},"spec":{"containers":[{"name":"app","customField":"preserved"}],
				"volumes":[{"name":"data","emptyDir":{}},{"name":"data","emptyDir":{}}]}}`,
			wantPod: `{"metadata":{"name":"test"},"spec":{"containers":[{"name":"app","customField":"preserved"}],
				"volumes":[{"name":"data","emptyDir":{}}]}}`,
		},
		{
			name: "normalization error does not prevent mutation",
			rawPod: `{"metadata":{"name":"test"},"spec":{"containers":[{"name":"app","customField":"preserved"}],
				"volumes":[{"name":"data","emptyDir":{}},{"name":"data","hostPath":{"path":"/data"}}]}}`,
			wantPod: `{"metadata":{"name":"test","annotations":{"injected":"true"}},"spec":{"containers":[{"name":"app","customField":"preserved","env":[{"name":"INJECTED_ENV","value":"true"}]}],
				"volumes":[{"name":"data","emptyDir":{}},{"name":"data","hostPath":{"path":"/data"}}]}}`,
			mutate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutator := func(pod *corev1.Pod, ns string, _ dynamic.Interface) (bool, error) {
				require.Equal(t, "application", ns)
				if !tt.mutate {
					return false, nil
				}
				AddAnnotation(pod, "injected", "true")
				return InjectEnv(pod, corev1.EnvVar{Name: "INJECTED_ENV", Value: "true"}), nil
			}

			// Keep the request raw so unknown fields survive until the patch is applied.
			rawPod := []byte(tt.rawPod)
			patchJSON, err := Mutate(rawPod, "application", "test", mutator, nil)
			require.NoError(t, err)
			patch, err := jsonpatch.DecodePatch(patchJSON)
			require.NoError(t, err)
			patchedJSON, err := patch.Apply(rawPod)
			require.NoError(t, err, "patch must apply to the original raw request")
			require.JSONEq(t, tt.wantPod, string(patchedJSON))
		})
	}
}

func Test_contains(t *testing.T) {
	type args struct {
		envs []corev1.EnvVar
		name string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "contains",
			args: args{
				envs: []corev1.EnvVar{
					{Name: "foo", Value: "bar"},
					{Name: "baz", Value: "bar"},
				},
				name: "baz",
			},
			want: true,
		},
		{
			name: "doesn't contain",
			args: args{
				envs: []corev1.EnvVar{
					{Name: "foo", Value: "bar"},
					{Name: "baz", Value: "bar"},
				},
				name: "baf",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contains(tt.args.envs, tt.args.name); got != tt.want {
				t.Errorf("contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_injectEnv(t *testing.T) {
	type args struct {
		pod *corev1.Pod
		env corev1.EnvVar
	}
	tests := []struct {
		name        string
		args        args
		wantPodFunc func() corev1.Pod
		injected    bool
	}{
		{
			name: "1 container, 1 inject env",
			args: args{
				pod: FakePodWithContainer("foo-pod", FakeContainer("foo-container")),
				env: fakeEnv("inject-me"),
			},
			wantPodFunc: func() corev1.Pod {
				pod := FakePodWithContainer("foo-pod", FakeContainer("foo-container"))
				pod.Spec.Containers[0].Env = append([]corev1.EnvVar{fakeEnv("inject-me")}, pod.Spec.Containers[0].Env...)
				return *pod
			},
			injected: true,
		},
		{
			name: "1 container, 0 inject env",
			args: args{
				pod: FakePodWithContainer("foo-pod", FakeContainer("foo-container")),
				env: fakeEnv("foo-container-env-foo"),
			},
			wantPodFunc: func() corev1.Pod {
				return *FakePodWithContainer("foo-pod", FakeContainer("foo-container"))
			},
			injected: false,
		},
		{
			name: "2 container, 2 inject env",
			args: args{
				pod: FakePodWithContainer("foo-pod", FakeContainer("foo-container"), FakeContainer("bar-container")),
				env: fakeEnv("inject-me"),
			},
			wantPodFunc: func() corev1.Pod {
				pod := FakePodWithContainer("foo-pod", FakeContainer("foo-container"), FakeContainer("bar-container"))
				pod.Spec.Containers[0].Env = append([]corev1.EnvVar{fakeEnv("inject-me")}, pod.Spec.Containers[0].Env...)
				pod.Spec.Containers[1].Env = append([]corev1.EnvVar{fakeEnv("inject-me")}, pod.Spec.Containers[1].Env...)
				return *pod
			},
			injected: true,
		},
		{
			name: "2 container, 1 inject env",
			args: args{
				pod: FakePodWithContainer("foo-pod", FakeContainer("foo-container"), FakeContainer("bar-container")),
				env: fakeEnv("foo-container-env-foo"),
			},
			wantPodFunc: func() corev1.Pod {
				pod := FakePodWithContainer("foo-pod", FakeContainer("foo-container"), FakeContainer("bar-container"))
				pod.Spec.Containers[1].Env = append([]corev1.EnvVar{fakeEnv("foo-container-env-foo")}, pod.Spec.Containers[1].Env...)
				return *pod
			},
			injected: true,
		},
		{
			name: "init containers",
			args: args{
				pod: fakePodWithInitContainer("foo-pod", FakeContainer("foo-init-container")),
				env: fakeEnv("inject-me"),
			},
			wantPodFunc: func() corev1.Pod {
				pod := fakePodWithInitContainer("foo-pod", FakeContainer("foo-init-container"))
				pod.Spec.InitContainers[0].Env = append([]corev1.EnvVar{fakeEnv("inject-me")}, pod.Spec.InitContainers[0].Env...)
				return *pod
			},
			injected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InjectEnv(tt.args.pod, tt.args.env)
			if got != tt.injected {
				t.Errorf("InjectEnv() = %v, want %v", got, tt.injected)
			}
			if tt.args.pod != nil && !reflect.DeepEqual(tt.args.pod.Spec.Containers, tt.wantPodFunc().Spec.Containers) {
				t.Errorf("InjectEnv() = %v, want %v", tt.args.pod.Spec.Containers, tt.wantPodFunc().Spec.Containers)
			}
		})
	}
}

func Test_addAnnotation(t *testing.T) {
	tests := []struct {
		name             string
		pod              *corev1.Pod
		key              string
		value            string
		expected         map[string]string
		expectedMutation bool
	}{
		{
			name:             "add annotation",
			pod:              FakePod("foo"),
			key:              "foo",
			value:            "bar",
			expected:         map[string]string{"foo": "bar"},
			expectedMutation: true,
		},
		{
			name:             "add annotation to existing",
			pod:              FakePodWithAnnotations(map[string]string{"foo": "bar"}),
			key:              "baz",
			value:            "qux",
			expected:         map[string]string{"foo": "bar", "baz": "qux"},
			expectedMutation: true,
		},
		{
			name:             "add annotation to existing with same key",
			pod:              FakePodWithAnnotations(map[string]string{"foo": "bar"}),
			key:              "foo",
			value:            "qux",
			expected:         map[string]string{"foo": "bar"},
			expectedMutation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AddAnnotation(tt.pod, tt.key, tt.value)
			if got != tt.expectedMutation {
				t.Errorf("AddAnnotation() = %v, want %v", got, tt.expectedMutation)
			}

			require.Equal(t, tt.expected, tt.pod.Annotations)
		})
	}

}

func Test_injectVolume(t *testing.T) {
	type args struct {
		pod         *corev1.Pod
		volume      corev1.Volume
		volumeMount corev1.VolumeMount
	}
	tests := []struct {
		name     string
		args     args
		injected bool
	}{
		{
			name: "nominal case",
			args: args{
				pod:         FakePod("foo"),
				volume:      corev1.Volume{Name: "volumefoo"},
				volumeMount: corev1.VolumeMount{Name: "volumefoo"},
			},
			injected: true,
		},
		{
			name: "volume exists",
			args: args{
				pod:         fakePodWithVolume("podfoo", "volumefoo", "/foo"),
				volume:      corev1.Volume{Name: "volumefoo"},
				volumeMount: corev1.VolumeMount{Name: "volumefoo"},
			},
			injected: false,
		},
		{
			name: "volume mount exists",
			args: args{
				pod:         fakePodWithVolume("podfoo", "volumefoo", "/foo"),
				volume:      corev1.Volume{Name: "differentName"},
				volumeMount: corev1.VolumeMount{Name: "volumefoo"},
			},
			injected: false,
		},
		{
			name: "mount path exists in one container",
			args: args{
				pod:         withContainer(fakePodWithVolume("podfoo", "volumefoo", "/foo"), "second-container"),
				volume:      corev1.Volume{Name: "differentName"},
				volumeMount: corev1.VolumeMount{Name: "volumefoo", MountPath: "/foo"},
			},
			injected: true,
		},
		{
			name: "mount path exists",
			args: args{
				pod:         fakePodWithVolume("podfoo", "volumefoo", "/foo"),
				volume:      corev1.Volume{Name: "differentName"},
				volumeMount: corev1.VolumeMount{Name: "differentName", MountPath: "/foo"},
			},
			injected: false,
		},
		{
			name: "mount path exists in one container",
			args: args{
				pod:         withContainer(fakePodWithVolume("podfoo", "volumefoo", "/foo"), "-second-container"),
				volume:      corev1.Volume{Name: "differentName"},
				volumeMount: corev1.VolumeMount{Name: "differentName", MountPath: "/foo"},
			},
			injected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			injectedVolume, injectedMount := InjectVolume(tt.args.pod, tt.args.volume, tt.args.volumeMount)
			assert.Equal(t, tt.injected, injectedVolume)
			if injectedMount {
				for _, container := range tt.args.pod.Spec.Containers {
					foundVolumeMount := false
					for _, vMount := range container.VolumeMounts {
						foundVolumeMount = foundVolumeMount || (vMount.MountPath == tt.args.volumeMount.MountPath)
					}
					assert.Truef(t, foundVolumeMount, "Expected finding volume mount path %q in container %q", tt.args.volumeMount.MountPath, container.Name)
				}
			}
		})
	}
}

func TestMarkVolumeAsSafeToEvictForAutoscaler(t *testing.T) {
	tests := []struct {
		name                                  string
		currentSafeToEvictAnnotationValue     string
		volumeToAdd                           string
		expectedNewSafeToEvictAnnotationValue string
	}{
		{
			name:                                  "the annotation is not set",
			currentSafeToEvictAnnotationValue:     "",
			volumeToAdd:                           "datadog",
			expectedNewSafeToEvictAnnotationValue: "datadog",
		},
		{
			name:                                  "the annotation is already set",
			currentSafeToEvictAnnotationValue:     "someVolume1,someVolume2",
			volumeToAdd:                           "datadog",
			expectedNewSafeToEvictAnnotationValue: "someVolume1,someVolume2,datadog",
		},
		{
			name:                                  "the annotation is already set and the volume is already in the list",
			currentSafeToEvictAnnotationValue:     "someVolume1,someVolume2",
			volumeToAdd:                           "someVolume2",
			expectedNewSafeToEvictAnnotationValue: "someVolume1,someVolume2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(_ *testing.T) {
			annotations := map[string]string{}
			if test.currentSafeToEvictAnnotationValue != "" {
				annotations["cluster-autoscaler.kubernetes.io/safe-to-evict-local-volumes"] = test.currentSafeToEvictAnnotationValue
			}
			pod := FakePodWithAnnotations(annotations)

			MarkVolumeAsSafeToEvictForAutoscaler(pod, test.volumeToAdd)

			assert.Equal(
				t,
				test.expectedNewSafeToEvictAnnotationValue,
				pod.Annotations["cluster-autoscaler.kubernetes.io/safe-to-evict-local-volumes"],
			)
		})
	}

}

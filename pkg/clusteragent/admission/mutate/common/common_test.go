// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/dynamic"
)

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

// Apply the admission patch to the original, unstructured request: decoding the
// result as a corev1.Pod would hide the very fields this test must preserve.
func TestMutatePreservesUnknownFields(t *testing.T) {
	const raw = `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"test"},"spec":{"futurePodField":{"enabled":true},"containers":[{"name":"app","image":"app:v1","securityContext":{"capabilities":{"ambient":["CHOWN"]}},"futureContainerField":"app"},{"name":"other","image":"other:v1","futureContainerField":"other"}],"initContainers":[{"name":"init","image":"init:v1","futureContainerField":"init"}]}}`
	tests := []struct {
		name   string
		mutate func(*corev1.Pod)
		want   func(map[string]interface{})
	}{
		{name: "no mutation", mutate: func(*corev1.Pod) {}, want: func(map[string]interface{}) {}},
		{name: "add resource requests with missing parent", mutate: func(p *corev1.Pod) {
			p.Spec.Containers[0].Resources.Requests = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")}
		}, want: func(spec map[string]interface{}) {
			spec["containers"].([]interface{})[0].(map[string]interface{})["resources"] = map[string]interface{}{"requests": map[string]interface{}{"cpu": "100m"}}
		}},
		{name: "edit known capability", mutate: func(p *corev1.Pod) {
			p.Spec.Containers[0].SecurityContext.Capabilities.Drop = []corev1.Capability{"ALL"}
		}, want: func(spec map[string]interface{}) {
			spec["containers"].([]interface{})[0].(map[string]interface{})["securityContext"].(map[string]interface{})["capabilities"].(map[string]interface{})["drop"] = []interface{}{"ALL"}
		}},
		{name: "prepend container", mutate: func(p *corev1.Pod) {
			p.Spec.Containers = append([]corev1.Container{{Name: "sidecar", Image: "sidecar:v1"}}, p.Spec.Containers...)
		}, want: func(spec map[string]interface{}) {
			spec["containers"] = append([]interface{}{map[string]interface{}{"name": "sidecar", "image": "sidecar:v1", "resources": map[string]interface{}{}}}, spec["containers"].([]interface{})...)
		}},
		{name: "reorder and mutate containers", mutate: func(p *corev1.Pod) {
			p.Spec.Containers[0], p.Spec.Containers[1] = p.Spec.Containers[1], p.Spec.Containers[0]
			p.Spec.Containers[1].Image = "app:v2"
		}, want: func(spec map[string]interface{}) {
			cs := spec["containers"].([]interface{})
			cs[0], cs[1] = cs[1], cs[0]
			cs[1].(map[string]interface{})["image"] = "app:v2"
		}},
		{name: "remove container", mutate: func(p *corev1.Pod) {
			p.Spec.Containers = p.Spec.Containers[1:]
		}, want: func(spec map[string]interface{}) {
			spec["containers"] = spec["containers"].([]interface{})[1:]
		}},
		{name: "prepend init container", mutate: func(p *corev1.Pod) {
			p.Spec.InitContainers = append([]corev1.Container{{Name: "setup", Image: "setup:v1"}}, p.Spec.InitContainers...)
		}, want: func(spec map[string]interface{}) {
			spec["initContainers"] = append([]interface{}{map[string]interface{}{"name": "setup", "image": "setup:v1", "resources": map[string]interface{}{}}}, spec["initContainers"].([]interface{})...)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patchBytes, err := Mutate([]byte(raw), "default", "test", func(p *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
				tt.mutate(p)
				return tt.name != "no mutation", nil
			}, nil)
			require.NoError(t, err)
			patch, err := jsonpatch.DecodePatch(patchBytes)
			require.NoError(t, err)
			actual, err := patch.Apply([]byte(raw))
			require.NoError(t, err)
			var expected map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(raw), &expected))
			tt.want(expected["spec"].(map[string]interface{}))
			want, err := json.Marshal(expected)
			require.NoError(t, err)
			assert.JSONEq(t, string(want), string(actual))
			if tt.name == "no mutation" {
				assert.Empty(t, patch)
			}
		})
	}
}

func TestMutatePreservesNormalization(t *testing.T) {
	raw := []byte(`{"metadata":{"name":"test"},"spec":{"containers":[{"name":"app","image":"app:v1"}],"volumes":[{"name":"data","emptyDir":{},"futureVolumeField":"data"},{"name":"data","emptyDir":{},"futureVolumeField":"data"},{"name":"cache","emptyDir":{},"futureVolumeField":"cache"}]}}`)
	patchBytes, err := Mutate(raw, "default", "test", func(*corev1.Pod, string, dynamic.Interface) (bool, error) { return false, nil }, nil)
	require.NoError(t, err)
	patch, err := jsonpatch.DecodePatch(patchBytes)
	require.NoError(t, err)
	result, err := patch.Apply(raw)
	require.NoError(t, err)
	var pod corev1.Pod
	require.NoError(t, json.Unmarshal(result, &pod))
	require.Len(t, pod.Spec.Volumes, 2)
	assert.Equal(t, "data", pod.Spec.Volumes[0].Name)
	assert.Equal(t, "cache", pod.Spec.Volumes[1].Name)
	var unstructured map[string]interface{}
	require.NoError(t, json.Unmarshal(result, &unstructured))
	volumes := unstructured["spec"].(map[string]interface{})["volumes"].([]interface{})
	assert.Equal(t, "data", volumes[0].(map[string]interface{})["futureVolumeField"])
	assert.Equal(t, "cache", volumes[1].(map[string]interface{})["futureVolumeField"])
}

func TestMutateFalseWithChanges(t *testing.T) {
	raw := []byte(`{"metadata":{"name":"test"},"spec":{"containers":[{"name":"app","image":"app:v1"}],"futurePodField":true}}`)
	patchBytes, err := Mutate(raw, "default", "test", func(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
		AddAnnotation(pod, "injection-status", "blocked")
		return false, nil
	}, nil)
	require.NoError(t, err)
	patch, err := jsonpatch.DecodePatch(patchBytes)
	require.NoError(t, err)
	result, err := patch.Apply(raw)
	require.NoError(t, err)
	assert.JSONEq(t, `{"metadata":{"name":"test","annotations":{"injection-status":"blocked"}},"spec":{"containers":[{"name":"app","image":"app:v1"}],"futurePodField":true}}`, string(result))
}

func BenchmarkMutate(b *testing.B) {
	for _, count := range []int{1, 100} {
		pod := corev1.Pod{}
		for i := 0; i < count; i++ {
			container := corev1.Container{Name: fmt.Sprintf("app-%d", i), Image: "app:v1"}
			for j := 0; j < 20; j++ {
				container.Env = append(container.Env, corev1.EnvVar{Name: fmt.Sprintf("VAR_%d", j), Value: "value"})
			}
			pod.Spec.Containers = append(pod.Spec.Containers, container)
		}
		raw, err := json.Marshal(pod)
		require.NoError(b, err)
		for _, mutation := range []bool{false, true} {
			b.Run(fmt.Sprintf("containers=%d/mutated=%t", count, mutation), func(b *testing.B) {
				mutator := func(p *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
					if mutation {
						p.Spec.Containers[0].Image = "app:v2"
					}
					return mutation, nil
				}
				b.ReportAllocs()
				b.SetBytes(int64(len(raw)))
				for b.Loop() {
					_, err := Mutate(raw, "default", "benchmark", mutator, nil)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

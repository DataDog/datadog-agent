// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
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
				volumeMount: corev1.VolumeMount{Name: "volumefoo"},
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
			assert.Equal(t, tt.injected, InjectVolume(tt.args.pod, tt.args.volume, tt.args.volumeMount))
		})
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package mutate

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func fakeEnvWithValue(name, value string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  name,
		Value: value,
	}
}

func fakeEnv(name string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  name,
		Value: name + "-env-value",
	}
}

func fakeContainer(name string) corev1.Container {
	return corev1.Container{
		Name: name,
		Env: []corev1.EnvVar{
			fakeEnv(name + "-env-foo"),
			fakeEnv(name + "-env-bar"),
		},
	}
}

func fakePodWithContainer(name string, containers ...corev1.Container) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.PodSpec{
			Containers: containers,
		},
	}
}

func fakePodWithLabel(k, v string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				k: v,
			},
		},
	}
}

func fakePodWithEnv(name, env string) *corev1.Pod {
	return fakePodWithContainer(name, corev1.Container{Name: name + "-container", Env: []corev1.EnvVar{fakeEnv(env)}})
}

func fakePod(name string) *corev1.Pod {
	return fakePodWithContainer(name, corev1.Container{Name: name + "-container"})
}

func boolPointer(b bool) *bool {
	return &b
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package common

import (
	"context"
	"fmt"
	"testing"

	"go.uber.org/fx"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// FakeEnvWithValue returns an env var with the given name and value
func FakeEnvWithValue(name, value string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  name,
		Value: value,
	}
}

// FakeEnvWithFieldRefValue returns an env var with the given name and field ref
// value
func FakeEnvWithFieldRefValue(name, value string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: value,
			},
		},
	}
}

func fakeEnv(name string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  name,
		Value: name + "-env-value",
	}
}

// FakeContainer returns a container with the given name
func FakeContainer(name string) corev1.Container {
	return corev1.Container{
		Name: name,
		Env: []corev1.EnvVar{
			fakeEnv(name + "-env-foo"),
			fakeEnv(name + "-env-bar"),
		},
	}
}

// FakePodWithContainer returns a pod with the given name and containers
func FakePodWithContainer(name string, containers ...corev1.Container) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.PodSpec{
			Containers: containers,
		},
	}
}

func fakePodWithInitContainer(name string, containers ...corev1.Container) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.PodSpec{
			InitContainers: containers,
		},
	}
}

// WithLabels sets the labels of the given pod
func WithLabels(pod *corev1.Pod, labels map[string]string) *corev1.Pod {
	pod.Labels = labels
	return pod
}

// FakePodWithLabel returns a pod with the given label set to the given value
func FakePodWithLabel(k, v string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				k: v,
			},
		},
	}
}

// FakePodWithAnnotation returns a pod with the given annotation set to the
// given value
func FakePodWithAnnotation(k, v string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod",
			Annotations: map[string]string{
				k: v,
			},
		},
	}
	return withContainer(pod, "-container")
}

// FakePodSpec describes a pod we are going to create.
type FakePodSpec struct {
	NS          string
	Name        string
	Labels      map[string]string
	Annotations map[string]string
	Envs        []corev1.EnvVar
	ParentKind  string
	ParentName  string
}

// Create makes a Pod from a FakePodSpec setting up sane defaults.
func (f FakePodSpec) Create() *corev1.Pod {
	if f.NS == "" {
		f.NS = "ns"
	}
	if f.Name == "" {
		f.Name = "pod"
	}
	return fakePodWithParent(f.NS, f.Name, f.Annotations, f.Labels, f.Envs, f.ParentKind, f.ParentName)
}

// fakePodWithParent returns a pod with the given parent kind and name
func fakePodWithParent(ns, name string, as, ls map[string]string, es []corev1.EnvVar, parentKind, parentName string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       ns,
			Annotations:     as,
			Labels:          ls,
			OwnerReferences: []metav1.OwnerReference{},
		},
	}
	pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
		Name: pod.Name,
		Env:  es,
	})

	var ownerRef *metav1.OwnerReference
	objMeta := metav1.ObjectMeta{
		Name:      parentName,
		Namespace: ns,
	}

	if parentKind == "replicaset" {
		rs := &appsv1.ReplicaSet{
			ObjectMeta: objMeta,
		}
		ownerRef = metav1.NewControllerRef(rs, appsv1.SchemeGroupVersion.WithKind("ReplicaSet"))
	} else if parentKind == "statefulset" {
		ss := &appsv1.StatefulSet{
			ObjectMeta: objMeta,
		}
		ownerRef = metav1.NewControllerRef(ss, appsv1.SchemeGroupVersion.WithKind("StatefulSet"))
	} else if parentKind == "job" {
		j := &batchv1.Job{
			ObjectMeta: objMeta,
		}
		ownerRef = metav1.NewControllerRef(j, batchv1.SchemeGroupVersion.WithKind("Job"))
	} else if parentKind == "daemonset" {
		ds := &appsv1.DaemonSet{
			ObjectMeta: objMeta,
		}
		ownerRef = metav1.NewControllerRef(ds, appsv1.SchemeGroupVersion.WithKind("DaemonSet"))
	} else {
		return pod
	}
	pod.ObjectMeta.OwnerReferences = append(pod.ObjectMeta.OwnerReferences, *ownerRef)

	return pod
}

// FakePodWithAnnotations returns a pod with the given annotations
func FakePodWithAnnotations(as map[string]string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pod",
			Annotations: as,
		},
	}
	return withContainer(pod, "-container")
}

// FakePodWithEnv returns a pod with the given env var
func FakePodWithEnv(name, env string) *corev1.Pod {
	return FakePodWithContainer(name, corev1.Container{Name: name + "-container", Env: []corev1.EnvVar{fakeEnv(env)}})
}

// FakePodWithEnvValue returns a pod with the given env var value
func FakePodWithEnvValue(name, envKey, envVal string) *corev1.Pod {
	return FakePodWithContainer(name, corev1.Container{Name: name + "-container", Env: []corev1.EnvVar{FakeEnvWithValue(envKey, envVal)}})
}

// FakePodWithEnvFieldRefValue returns a pod with the given env var field ref
func FakePodWithEnvFieldRefValue(name, envKey, path string) *corev1.Pod {
	return FakePodWithContainer(name, corev1.Container{Name: name + "-container", Env: []corev1.EnvVar{FakeEnvWithFieldRefValue(envKey, path)}})
}

// FakePodWithNamespaceAndLabel returns a pod with the given label and namespace
func FakePodWithNamespaceAndLabel(namespace, k, v string) *corev1.Pod {
	pod := FakePodWithLabel(k, v)
	pod.Namespace = namespace

	return pod
}

func fakePodWithVolume(podName, volumeName, mountPath string) *corev1.Pod {
	pod := FakePod(podName)
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: volumeName})
	pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{Name: volumeName, MountPath: mountPath})
	return pod
}

// FakePod returns a pod with the given name
func FakePod(name string) *corev1.Pod {
	return FakePodWithContainer(name, corev1.Container{Name: name + "-container"})
}

// FakePodWithResources with resource requirements
func FakePodWithResources(name string, reqs corev1.ResourceRequirements) *corev1.Pod {
	return FakePodWithContainer(name, corev1.Container{Name: name + "-container", Resources: reqs})
}

func withContainer(pod *corev1.Pod, nameSuffix string) *corev1.Pod {
	pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: pod.Name + nameSuffix})
	return pod
}

// MockDeployment represents a deployment to be used in tests
type MockDeployment struct {
	ContainerName   string
	DeploymentName  string
	Namespace       string
	IsInitContainer bool
	Languages       util.LanguageSet
}

// FakeStoreWithDeployment sets up a fake workloadmeta with the given
// deployments
func FakeStoreWithDeployment(t *testing.T, deployments []MockDeployment) workloadmeta.Component {
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		coreconfig.MockModule(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	for _, d := range deployments {
		langSet := util.LanguageSet{}
		for lang := range d.Languages {
			langSet.Add(lang)
		}
		container := util.Container{
			Name: d.ContainerName,
			Init: d.IsInitContainer,
		}

		mockStore.Set(&workloadmeta.KubernetesDeployment{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesDeployment,
				ID:   fmt.Sprintf("%s/%s", d.Namespace, d.DeploymentName),
			},
			InjectableLanguages: util.ContainersLanguages{
				container: langSet,
			},
		})
	}

	return mockStore
}

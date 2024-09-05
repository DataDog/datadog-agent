// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package config

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	admiv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	admCommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func Test_injectionMode(t *testing.T) {
	tests := []struct {
		name       string
		pod        *corev1.Pod
		globalMode string
		want       string
	}{
		{
			name:       "nominal case",
			pod:        mutatecommon.FakePod("foo"),
			globalMode: "hostip",
			want:       "hostip",
		},
		{
			name:       "custom mode #1",
			pod:        mutatecommon.FakePodWithLabel("admission.datadoghq.com/config.mode", "service"),
			globalMode: "hostip",
			want:       "service",
		},
		{
			name:       "custom mode #2",
			pod:        mutatecommon.FakePodWithLabel("admission.datadoghq.com/config.mode", "socket"),
			globalMode: "hostip",
			want:       "socket",
		},
		{
			name:       "invalid",
			pod:        mutatecommon.FakePodWithLabel("admission.datadoghq.com/config.mode", "wrong"),
			globalMode: "hostip",
			want:       "hostip",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, injectionMode(tt.pod, tt.globalMode))
		})
	}
}

func TestInjectHostIP(t *testing.T) {
	pod := mutatecommon.FakePodWithContainer("foo-pod", corev1.Container{})
	pod = mutatecommon.WithLabels(pod, map[string]string{"admission.datadoghq.com/enabled": "true"})
	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
	webhook := NewWebhook(wmeta, autoinstrumentation.GetInjectionFilter())
	injected, err := webhook.inject(pod, "", nil)
	assert.Nil(t, err)
	assert.True(t, injected)
	assert.Contains(t, pod.Spec.Containers[0].Env, mutatecommon.FakeEnvWithFieldRefValue("DD_AGENT_HOST", "status.hostIP"))
}

func TestInjectService(t *testing.T) {
	pod := mutatecommon.FakePodWithContainer("foo-pod", corev1.Container{})
	pod = mutatecommon.WithLabels(pod, map[string]string{"admission.datadoghq.com/enabled": "true", "admission.datadoghq.com/config.mode": "service"})
	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
	webhook := NewWebhook(wmeta, autoinstrumentation.GetInjectionFilter())
	injected, err := webhook.inject(pod, "", nil)
	assert.Nil(t, err)
	assert.True(t, injected)
	assert.Contains(t, pod.Spec.Containers[0].Env, mutatecommon.FakeEnvWithValue("DD_AGENT_HOST", "datadog."+common.GetMyNamespace()+".svc.cluster.local"))
}

func TestInjectEntityID(t *testing.T) {
	for _, tt := range []struct {
		name            string
		env             corev1.EnvVar
		configOverrides map[string]interface{}
	}{
		{
			name: "inject pod uid",
			env:  mutatecommon.FakeEnvWithFieldRefValue("DD_ENTITY_ID", "metadata.uid"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod := mutatecommon.FakePodWithContainer("foo-pod", corev1.Container{
				Name: "cont-name",
			})
			pod = mutatecommon.WithLabels(pod, map[string]string{"admission.datadoghq.com/enabled": "true"})
			wmeta := fxutil.Test[workloadmeta.Component](
				t,
				core.MockBundle(),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
				fx.Replace(config.MockParams{Overrides: tt.configOverrides}),
			)
			webhook := NewWebhook(wmeta, autoinstrumentation.GetInjectionFilter())
			injected, err := webhook.inject(pod, "", nil)
			assert.Nil(t, err)
			assert.True(t, injected)
			assert.Contains(t, pod.Spec.Containers[0].Env, tt.env)
		})
	}
}

func TestInjectExternalDataEnvVar(t *testing.T) {
	testCases := []struct {
		name          string
		inputPod      corev1.Pod
		expectedPod   corev1.Pod
		expectedValue bool
	}{
		{
			name: "normal case",
			inputPod: corev1.Pod{
				Spec: corev1.PodSpec{
					Containers:     []corev1.Container{{Name: "cont-name"}},
					InitContainers: []corev1.Container{{Name: "init-container"}},
				},
			},
			expectedPod: corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "cont-name",
						Env: []corev1.EnvVar{
							{Name: podUIDEnvVarName, ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"}}},
							{Name: ddExternalDataEnvVarName, Value: "it-false,cn-cont-name,pu-$(DD_INTERNAL_POD_UID)"},
						},
					}},
					InitContainers: []corev1.Container{{
						Name: "init-container",
						Env: []corev1.EnvVar{
							{Name: podUIDEnvVarName, ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"}}},
							{Name: ddExternalDataEnvVarName, Value: "it-true,cn-init-container,pu-$(DD_INTERNAL_POD_UID)"},
						},
					}},
				},
			},
			expectedValue: true,
		},
		{
			name: "multiple containers",
			inputPod: corev1.Pod{
				Spec: corev1.PodSpec{
					Containers:     []corev1.Container{{Name: "cont-name-1"}, {Name: "cont-name-2"}, {Name: "cont-name-3"}},
					InitContainers: []corev1.Container{{Name: "init-container-1"}, {Name: "init-container-2"}, {Name: "init-container-3"}},
				},
			},
			expectedPod: corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "cont-name-1",
						Env: []corev1.EnvVar{
							{Name: podUIDEnvVarName, ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"}}},
							{Name: ddExternalDataEnvVarName, Value: "it-false,cn-cont-name-1,pu-$(DD_INTERNAL_POD_UID)"},
						},
					}, {
						Name: "cont-name-2",
						Env: []corev1.EnvVar{
							{Name: podUIDEnvVarName, ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"}}},
							{Name: ddExternalDataEnvVarName, Value: "it-false,cn-cont-name-2,pu-$(DD_INTERNAL_POD_UID)"},
						},
					}, {
						Name: "cont-name-3",
						Env: []corev1.EnvVar{
							{Name: podUIDEnvVarName, ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"}}},
							{Name: ddExternalDataEnvVarName, Value: "it-false,cn-cont-name-3,pu-$(DD_INTERNAL_POD_UID)"},
						},
					}},
					InitContainers: []corev1.Container{{
						Name: "init-container-1",
						Env: []corev1.EnvVar{
							{Name: podUIDEnvVarName, ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"}}},
							{Name: ddExternalDataEnvVarName, Value: "it-true,cn-init-container-1,pu-$(DD_INTERNAL_POD_UID)"},
						},
					}, {
						Name: "init-container-2",
						Env: []corev1.EnvVar{
							{Name: podUIDEnvVarName, ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"}}},
							{Name: ddExternalDataEnvVarName, Value: "it-true,cn-init-container-2,pu-$(DD_INTERNAL_POD_UID)"},
						},
					}, {
						Name: "init-container-3",
						Env: []corev1.EnvVar{
							{Name: podUIDEnvVarName, ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"}}},
							{Name: ddExternalDataEnvVarName, Value: "it-true,cn-init-container-3,pu-$(DD_INTERNAL_POD_UID)"},
						},
					}},
				},
			},
			expectedValue: true,
		},
		{
			name: "with only normal containers",
			inputPod: corev1.Pod{
				Spec: corev1.PodSpec{
					Containers:     []corev1.Container{{Name: "cont-name"}},
					InitContainers: []corev1.Container{},
				},
			},
			expectedPod: corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "cont-name",
						Env: []corev1.EnvVar{
							{Name: podUIDEnvVarName, ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"}}},
							{Name: ddExternalDataEnvVarName, Value: "it-false,cn-cont-name,pu-$(DD_INTERNAL_POD_UID)"},
						},
					}},
					InitContainers: []corev1.Container{},
				},
			},
			expectedValue: true,
		},
		{
			name: "with only init containers",
			inputPod: corev1.Pod{
				Spec: corev1.PodSpec{
					Containers:     []corev1.Container{},
					InitContainers: []corev1.Container{{Name: "init-container"}},
				},
			},
			expectedPod: corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{},
					InitContainers: []corev1.Container{{
						Name: "init-container",
						Env: []corev1.EnvVar{
							{Name: podUIDEnvVarName, ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"}}},
							{Name: ddExternalDataEnvVarName, Value: "it-true,cn-init-container,pu-$(DD_INTERNAL_POD_UID)"},
						},
					}},
				},
			},
			expectedValue: true,
		},
		{
			name: "with nil containers",
			inputPod: corev1.Pod{
				Spec: corev1.PodSpec{
					Containers:     []corev1.Container{},
					InitContainers: []corev1.Container{},
				},
			},
			expectedPod: corev1.Pod{
				Spec: corev1.PodSpec{
					Containers:     []corev1.Container{},
					InitContainers: []corev1.Container{},
				},
			},
			expectedValue: false,
		},
		{
			name:          "with nil pod",
			inputPod:      corev1.Pod{},
			expectedPod:   corev1.Pod{},
			expectedValue: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := injectExternalDataEnvVar(&tc.inputPod)
			assert.Equal(t, tc.expectedValue, got)
			assert.Equal(t, tc.expectedPod, tc.inputPod)
		})
	}
}

func TestInjectSocket(t *testing.T) {
	pod := mutatecommon.FakePodWithContainer("foo-pod", corev1.Container{})
	pod = mutatecommon.WithLabels(pod, map[string]string{"admission.datadoghq.com/enabled": "true", "admission.datadoghq.com/config.mode": "socket"})
	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
	webhook := NewWebhook(wmeta, autoinstrumentation.GetInjectionFilter())
	injected, err := webhook.inject(pod, "", nil)
	assert.Nil(t, err)
	assert.True(t, injected)
	assert.Contains(t, pod.Spec.Containers[0].Env, mutatecommon.FakeEnvWithValue("DD_TRACE_AGENT_URL", "unix:///var/run/datadog/apm.socket"))
	assert.Contains(t, pod.Spec.Containers[0].Env, mutatecommon.FakeEnvWithValue("DD_DOGSTATSD_URL", "unix:///var/run/datadog/dsd.socket"))
	assert.Equal(t, pod.Spec.Containers[0].VolumeMounts[0].MountPath, "/var/run/datadog")
	assert.Equal(t, pod.Spec.Containers[0].VolumeMounts[0].Name, "datadog")
	assert.Equal(t, pod.Spec.Containers[0].VolumeMounts[0].ReadOnly, true)
	assert.Equal(t, pod.Spec.Volumes[0].Name, "datadog")
	assert.Equal(t, pod.Spec.Volumes[0].VolumeSource.HostPath.Path, "/var/run/datadog")
	assert.Equal(t, *pod.Spec.Volumes[0].VolumeSource.HostPath.Type, corev1.HostPathDirectoryOrCreate)
	assert.Equal(t, "datadog", pod.Annotations[mutatecommon.K8sAutoscalerSafeToEvictVolumesAnnotation])
}

func TestInjectSocketWithConflictingVolumeAndInitContainer(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Labels: map[string]string{
				"admission.datadoghq.com/enabled":     "true",
				"admission.datadoghq.com/config.mode": "socket",
			},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name: "init",
				},
			},
			Containers: []corev1.Container{
				{
					Name: "foo",
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "foo",
							MountPath: "/var/run/datadog",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "foo",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/var/run/datadog",
							Type: pointer.Ptr(corev1.HostPathDirectoryOrCreate),
						},
					},
				},
			},
		},
	}

	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
	webhook := NewWebhook(wmeta, autoinstrumentation.GetInjectionFilter())
	injected, err := webhook.inject(pod, "", nil)
	assert.True(t, injected)
	assert.Nil(t, err)
	assert.Equal(t, len(pod.Spec.InitContainers), 1)
	assert.Equal(t, pod.Spec.InitContainers[0].VolumeMounts[0].Name, "datadog")
	assert.Equal(t, pod.Spec.InitContainers[0].VolumeMounts[0].MountPath, "/var/run/datadog")
	assert.Equal(t, pod.Spec.InitContainers[0].VolumeMounts[0].ReadOnly, true)
	assert.Equal(t, len(pod.Spec.Volumes), 2)
	assert.Equal(t, pod.Spec.Volumes[1].Name, "datadog")
	assert.Equal(t, pod.Spec.Volumes[1].VolumeSource.HostPath.Path, "/var/run/datadog")
	assert.Equal(t, *pod.Spec.Volumes[1].VolumeSource.HostPath.Type, corev1.HostPathDirectoryOrCreate)
}

func TestJSONPatchCorrectness(t *testing.T) {
	for _, tt := range []struct {
		name      string
		file      string
		overrides map[string]interface{}
	}{
		{
			name: "inject only pod uid",
			file: "./testdata/expected_jsonpatch.json",
		},
		{
			name: "inject pod uid and cont_name",
			file: "./testdata/expected_jsonpatch_with_cont_name.json",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod := mutatecommon.FakePodWithContainer("foo", mutatecommon.FakeContainer("container"))
			mutatecommon.WithLabels(pod, map[string]string{admCommon.EnabledLabelKey: "true"})
			podJSON, err := json.Marshal(pod)
			assert.NoError(t, err)
			wmeta := fxutil.Test[workloadmeta.Component](t,
				core.MockBundle(),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
				fx.Replace(config.MockParams{Overrides: tt.overrides}),
			)
			webhook := NewWebhook(wmeta, autoinstrumentation.GetInjectionFilter())
			request := admission.MutateRequest{
				Raw:       podJSON,
				Namespace: "bar",
			}
			jsonPatch, err := webhook.MutateFunc()(&request)
			assert.NoError(t, err)

			expected, err := os.ReadFile(tt.file)
			assert.NoError(t, err)
			assert.JSONEq(t, string(expected), string(jsonPatch))
		})
	}
}

func BenchmarkJSONPatch(b *testing.B) {
	scheme := runtime.NewScheme()
	_ = admiv1.AddToScheme(scheme)
	decoder := serializer.NewCodecFactory(scheme).UniversalDeserializer()

	content, err := os.ReadFile("./testdata/large_pod.json")
	if err != nil {
		b.Fatal(err)
	}

	obj, _, err := decoder.Decode(content, nil, nil)
	if err != nil {
		b.Fatal(err)
	}

	wmeta := fxutil.Test[workloadmeta.Component](b, core.MockBundle())
	webhook := NewWebhook(wmeta, autoinstrumentation.GetInjectionFilter())
	podJSON := obj.(*admiv1.AdmissionReview).Request.Object.Raw

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		request := admission.MutateRequest{
			Raw:       podJSON,
			Namespace: "bar",
		}
		jsonPatch, err := webhook.MutateFunc()(&request)
		if err != nil {
			b.Fatal(err)
		}

		if len(jsonPatch) < 100 {
			b.Fatal("Empty JSONPatch")
		}
	}
}

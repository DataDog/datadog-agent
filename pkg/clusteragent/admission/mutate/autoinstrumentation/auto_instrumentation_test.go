// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const commonRegistry = "gcr.io/datadoghq"

var (
	defaultLibraries = map[string]string{
		"java":   "v1",
		"python": "v3",
		"ruby":   "v2",
		"dotnet": "v3",
		"js":     "v5",
		"php":    "v1",
	}

	// TODO: Add new entry when a new language is supported
	defaultLibImageVersions = map[language]string{
		java:   "registry/dd-lib-java-init:" + defaultLibraries["java"],
		js:     "registry/dd-lib-js-init:" + defaultLibraries["js"],
		python: "registry/dd-lib-python-init:" + defaultLibraries["python"],
		dotnet: "registry/dd-lib-dotnet-init:" + defaultLibraries["dotnet"],
		ruby:   "registry/dd-lib-ruby-init:" + defaultLibraries["ruby"],
		php:    "registry/dd-lib-php-init:" + defaultLibraries["php"],
	}
)

func defaultLibInfo(l language) libInfo {
	return libInfo{lang: l, image: defaultLibImageVersions[l]}
}

func defaultLibrariesFor(languages ...string) map[string]string {
	out := map[string]string{}
	for _, l := range languages {
		out[l] = defaultLibraries[l]
	}
	return out
}

func TestInjectAutoInstruConfigV2(t *testing.T) {
	buildRequireEnv := func(c corev1.Container) func(t *testing.T, k string, ok bool, val string) {
		envsByName := map[string]corev1.EnvVar{}
		for _, env := range c.Env {
			envsByName[env.Name] = env
		}

		return func(t *testing.T, key string, ok bool, value string) {
			t.Helper()
			val, exists := envsByName[key]
			require.Equal(t, ok, exists, "expected env %v exists to = %v", key, ok)
			require.Equal(t, value, val.Value, "expected env %v = %v", key, val)
		}
	}

	tests := []struct {
		name                                    string
		withWmeta                               func(wmeta workloadmetamock.Mock)
		pod                                     *corev1.Pod
		libInfo                                 extractedPodLibInfo
		expectedInjectorImage                   string
		expectedLangsDetected                   string
		expectedInstallType                     string
		expectedSecurityContext                 *corev1.SecurityContext
		expectedSecurityContextDoesNotSetConfig bool
		wantErr                                 bool
		config                                  func(c model.Config)
		expectedLdPreload                       string
		expectedLibConfigEnvs                   map[string]string
		assertExtraContainer                    func(*testing.T, corev1.Container)
	}{
		{
			name: "no libs, no injection",
			pod:  common.FakePod("java-pod"),
		},
		{
			name:                  "nominal case: java",
			pod:                   common.FakePod("java-pod"),
			expectedInjectorImage: commonRegistry + "/apm-inject:0",
			libInfo: extractedPodLibInfo{
				libs: []libInfo{
					java.libInfo("", "gcr.io/datadoghq/dd-lib-java-init:v1"),
				},
			},
		},
		{
			name:                  "nominal case: java & python",
			pod:                   common.FakePod("java-pod"),
			expectedInjectorImage: commonRegistry + "/apm-inject:0",
			libInfo: extractedPodLibInfo{
				libs: []libInfo{
					java.libInfo("", "gcr.io/datadoghq/dd-lib-java-init:v1"),
					python.libInfo("", "gcr.io/datadoghq/dd-lib-python-init:v1"),
				},
			},
		},
		{
			name: "java + injector-tag-override",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/apm-inject.version": "v0",
				},
			}.Create(),
			expectedInjectorImage: commonRegistry + "/apm-inject:v0",
			libInfo: extractedPodLibInfo{
				libs: []libInfo{
					java.libInfo("", "gcr.io/datadoghq/dd-lib-java-init:v1"),
				},
			},
		},
		{
			name: "java + injector-image-override",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/apm-inject.custom-image": "docker.io/library/apm-inject-package:v27",
				},
			}.Create(),
			expectedInjectorImage: "docker.io/library/apm-inject-package:v27",
			libInfo: extractedPodLibInfo{
				libs: []libInfo{
					java.libInfo("", "gcr.io/datadoghq/dd-lib-java-init:v1"),
				},
			},
		},
		{
			name: "java + debug enabled",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/apm-inject.version": "v0",
					"admission.datadoghq.com/apm-inject.debug":   "true",
				},
			}.Create(),
			expectedInjectorImage: commonRegistry + "/apm-inject:v0",
			expectedLibConfigEnvs: map[string]string{
				"DD_APM_INSTRUMENTATION_DEBUG": "true",
				"DD_TRACE_STARTUP_LOGS":        "true",
				"DD_TRACE_DEBUG":               "true",
			},
			libInfo: extractedPodLibInfo{
				libs: []libInfo{
					java.libInfo("", "gcr.io/datadoghq/dd-lib-java-init:v1"),
				},
			},
		},
		{
			name: "java + debug disabled",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/apm-inject.version": "v0",
					"admission.datadoghq.com/apm-inject.debug":   "false",
				},
			}.Create(),
			expectedInjectorImage: commonRegistry + "/apm-inject:v0",
			libInfo: extractedPodLibInfo{
				libs: []libInfo{
					java.libInfo("", "gcr.io/datadoghq/dd-lib-java-init:v1"),
				},
			},
		},
		{
			name:                  "config injector-image-override",
			pod:                   common.FakePod("java-pod"),
			expectedInjectorImage: "gcr.io/datadoghq/apm-inject:0.16-1",
			libInfo: extractedPodLibInfo{
				libs: []libInfo{
					java.libInfo("", "gcr.io/datadoghq/dd-lib-java-init:v1"),
				},
			},
			config: func(c model.Config) {
				c.SetWithoutSource("apm_config.instrumentation.injector_image_tag", "0.16-1")
			},
		},
		{
			name:                  "config language detected env vars",
			pod:                   common.FakePod("java-pod"),
			expectedInjectorImage: "gcr.io/datadoghq/apm-inject:0.16-1",
			expectedLangsDetected: "python",
			expectedInstallType:   "k8s_lib_injection",
			libInfo: extractedPodLibInfo{
				languageDetection: &libInfoLanguageDetection{
					libs: []libInfo{
						python.defaultLibInfo(commonRegistry, "java-pod-container"),
					},
				},
				libs: []libInfo{
					java.libInfo("", "gcr.io/datadoghq/dd-lib-java-init:v1"),
				},
				source: libInfoSourceLibInjection,
			},
			config: func(c model.Config) {
				c.SetWithoutSource("apm_config.instrumentation.injector_image_tag", "0.16-1")
			},
		},
		{
			name:                  "language detected for a different container",
			pod:                   common.FakePod("java-pod"),
			expectedInjectorImage: "gcr.io/datadoghq/apm-inject:0",
			expectedLangsDetected: "",
			libInfo: extractedPodLibInfo{
				languageDetection: &libInfoLanguageDetection{
					libs: []libInfo{
						python.defaultLibInfo(commonRegistry, "not-java-pod-container"),
					},
				},
				libs: []libInfo{
					java.libInfo("", "gcr.io/datadoghq/dd-lib-java-init:v1"),
				},
			},
		},
		{
			name:                  "language detected but no languages found",
			pod:                   common.FakePod("java-pod"),
			expectedInjectorImage: "gcr.io/datadoghq/apm-inject:0",
			expectedLangsDetected: "",
			libInfo: extractedPodLibInfo{
				languageDetection: &libInfoLanguageDetection{},
				libs: []libInfo{
					java.libInfo("", "gcr.io/datadoghq/dd-lib-java-init:v1"),
				},
			},
		},
		{
			name:                  "with specified install type and container security context",
			pod:                   common.FakePod("java-pod"),
			expectedInjectorImage: "gcr.io/datadoghq/apm-inject:0.16-1",
			expectedSecurityContext: &corev1.SecurityContext{
				Privileged: pointer.Ptr(false),
			},
			expectedLangsDetected: "python",
			libInfo: extractedPodLibInfo{
				languageDetection: &libInfoLanguageDetection{
					libs: []libInfo{
						python.defaultLibInfo(commonRegistry, "java-pod-container"),
					},
				},
				libs: []libInfo{
					java.libInfo("", "gcr.io/datadoghq/dd-lib-java-init:v1"),
				},
				source: libInfoSourceSingleStepLangaugeDetection,
			},
			config: func(c model.Config) {
				c.SetWithoutSource("apm_config.instrumentation.injector_image_tag", "0.16-1")
			},
		},
		{
			name:                  "with already set LD_PRELOAD",
			pod:                   common.FakePodWithEnvValue("python-pod", "LD_PRELOAD", "/foo/bar"),
			expectedInjectorImage: "gcr.io/datadoghq/apm-inject:0.16-1",
			expectedSecurityContext: &corev1.SecurityContext{
				Privileged: pointer.Ptr(false),
			},
			expectedLangsDetected: "python",
			libInfo: extractedPodLibInfo{
				languageDetection: &libInfoLanguageDetection{
					libs: []libInfo{
						python.defaultLibInfo(commonRegistry, "python-pod-container"),
					},
				},
				libs: []libInfo{
					python.libInfo("", "gcr.io/datadoghq/dd-lib-python-init:v1"),
				},
				source: libInfoSourceSingleStepLangaugeDetection,
			},
			config: func(c model.Config) {
				c.SetWithoutSource("apm_config.instrumentation.injector_image_tag", "0.16-1")
			},
			expectedLdPreload: "/foo/bar:/opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so",
		},
		{
			name: "all-lib.config",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.config.v1": `{"version":1,"runtime_metrics_enabled":true,"tracing_rate_limit":50,"tracing_sampling_rate":0.3}`,
				},
			}.Create(),
			libInfo: extractedPodLibInfo{
				libs: []libInfo{
					python.libInfo("", "gcr.io/datadoghq/dd-lib-python-init:v1"),
				},
			},
			expectedInjectorImage: "gcr.io/datadoghq/apm-inject:0",
			expectedLibConfigEnvs: map[string]string{
				"DD_RUNTIME_METRICS_ENABLED": "true",
				"DD_TRACE_RATE_LIMIT":        "50",
				"DD_TRACE_SAMPLE_RATE":       "0.30",
			},
		},
		{
			name: "app-container.config",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/python-lib.config.v1": `{"version":1,"runtime_metrics_enabled":true,"tracing_rate_limit":50,"tracing_sampling_rate":0.3}`,
					"admission.datadoghq.com/java-lib.config.v1":   `{"version":1,"runtime_metrics_enabled":false,"tracing_rate_limit":60,"tracing_sampling_rate":0.3}`,
				},
			}.Create(),
			libInfo: extractedPodLibInfo{
				libs: []libInfo{
					python.libInfo("", "gcr.io/datadoghq/dd-lib-python-init:v1"),
				},
			},
			expectedInjectorImage: "gcr.io/datadoghq/apm-inject:0",
			expectedLibConfigEnvs: map[string]string{
				"DD_RUNTIME_METRICS_ENABLED": "true",
				"DD_TRACE_RATE_LIMIT":        "50",
				"DD_TRACE_SAMPLE_RATE":       "0.30",
			},
		},
		{
			name: "istio-proxy",
			pod: common.FakePodSpec{
				Containers: []corev1.Container{{Name: "istio-proxy"}},
			}.Create(),
			expectedInjectorImage: commonRegistry + "/apm-inject:0",
			libInfo: extractedPodLibInfo{
				libs: []libInfo{
					java.libInfo("", "gcr.io/datadoghq/dd-lib-java-init:v1"),
				},
			},
			assertExtraContainer: func(t *testing.T, c corev1.Container) {
				t.Helper()
				requireEnv := buildRequireEnv(c)
				require.Equal(t, 0, len(c.VolumeMounts), "expected no volume mounts")
				requireEnv(t, "LD_PRELOAD", false, "")
				require.Equal(t, corev1.Container{Name: "istio-proxy"}, c, "container should be untouched")
			},
		},
		{
			name: "restricted security context",
			pod: common.FakePodSpec{
				NS: "restricted",
			}.Create(),
			withWmeta: func(wmeta workloadmetamock.Mock) {
				wmeta.Set(&workloadmeta.KubernetesMetadata{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesMetadata,
						ID:   "restricted",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "restricted",
						Labels: map[string]string{
							"pod-security.kubernetes.io/enforce": "restricted",
						},
					},
				})
			},
			expectedInjectorImage:                   commonRegistry + "/apm-inject:0",
			expectedSecurityContext:                 defaultRestrictedSecurityContext,
			expectedSecurityContextDoesNotSetConfig: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wmeta := common.FakeStoreWithDeployment(t, nil)
			if tt.withWmeta != nil {
				tt.withWmeta(wmeta.(workloadmetamock.Mock))
			}

			mockConfig := configmock.New(t)

			mockConfig.SetWithoutSource("apm_config.instrumentation.version", "v2")
			if tt.config != nil {
				tt.config(mockConfig)
			}

			config, err := NewConfig(mockConfig)
			require.NoError(t, err)

			require.Equal(t, instrumentationV2, config.version)
			require.True(t, config.version.usesInjector())

			if !tt.expectedSecurityContextDoesNotSetConfig {
				config.initSecurityContext = tt.expectedSecurityContext
			}

			if tt.libInfo.source == libInfoSourceNone {
				tt.libInfo.source = libInfoSourceSingleStepInstrumentation
			}

			if tt.expectedInstallType == "" {
				tt.expectedInstallType = "k8s_single_step"
			}

			mutator, err := NewNamespaceMutator(config, wmeta)
			require.NoError(t, err)

			err = mutator.core.injectTracers(tt.pod, tt.libInfo)
			if tt.wantErr {
				require.Error(t, err, "expected injectAutoInstruConfig to error")
			} else {
				require.NoError(t, err, "expected injectAutoInstruConfig to succeed")
			}

			if err != nil {
				return
			}

			requireEnv := buildRequireEnv(tt.pod.Spec.Containers[0])
			for _, env := range injectAllEnvs() {
				requireEnv(t, env.Name, false, "")
			}

			if len(tt.libInfo.libs) == 0 {
				require.Zero(t, len(tt.pod.Spec.InitContainers), "no libs, no init containers")
				requireEnv(t, "LD_PRELOAD", false, "")
				return
			}

			require.Equal(t, volumeName, tt.pod.Spec.Volumes[0].Name,
				"expected datadog volume to be injected")

			require.Equal(t, etcVolume.Name, tt.pod.Spec.Volumes[1].Name,
				"expected datadog-etc volume to be injected")

			volumesMarkedAsSafeToEvict := strings.Split(tt.pod.Annotations[common.K8sAutoscalerSafeToEvictVolumesAnnotation], ",")
			require.Contains(t, volumesMarkedAsSafeToEvict, volumeName, "expected volume %s to be marked as safe to evict", volumeName)
			require.Contains(t, volumesMarkedAsSafeToEvict, etcVolume.Name, "expected volume %s to be marked as safe to evict", etcVolume.Name)

			require.Equal(t, len(tt.libInfo.libs)+1, len(tt.pod.Spec.InitContainers),
				"expected there to be one more than the number of libs to inject for init containers")

			for i, c := range tt.pod.Spec.InitContainers {

				require.Equal(t, tt.expectedSecurityContext, c.SecurityContext,
					"expected %s.SecurityContext to be set", c.Name)

				var injectorMountPath string

				if i == 0 { // check inject container
					require.Equal(t, "datadog-init-apm-inject", c.Name,
						"expected the first init container to be apm-inject")
					require.Equal(t, tt.expectedInjectorImage, c.Image,
						"expected the container image to be %s", tt.expectedInjectorImage)
					injectorMountPath = c.VolumeMounts[0].MountPath
				} else { // lib volumes for each of the rest of the containers
					lib := tt.libInfo.libs[i-1]
					require.Equal(t, lib.image, c.Image)
					require.Equal(t, "opt/datadog/apm/library/"+string(lib.lang), c.VolumeMounts[0].SubPath,
						"expected a language specific sub-path for the volume mount for lang %s",
						lib.lang)
					require.Equal(t, "opt/datadog-packages/datadog-apm-inject", c.VolumeMounts[1].SubPath,
						"expected injector volume mount for lang %s",
						lib.lang)
					injectorMountPath = c.VolumeMounts[1].MountPath
				}

				// each of the init containers writes a timestamp to their given volume mount path
				require.Equal(t, 1, len(c.Args), "expected container args")
				// the last part of each of the init container's command should be writing
				// a timestamp based on the name of the container.
				expectedTimestampPath := injectorMountPath + "/c-init-time." + c.Name
				cmdTail := "&& echo $(date +%s) >> " + expectedTimestampPath
				require.Contains(t, c.Args[0], cmdTail, "expected args to contain %s", cmdTail)
				prefix, found := strings.CutSuffix(c.Args[0], cmdTail)
				require.True(t, found, "expected args to end with %s ", cmdTail)

				if i == 0 { // inject container
					require.Contains(t, prefix, "&& echo /opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so > /datadog-etc/ld.so.preload")
				}
			}

			// three volume mounts
			mounts := tt.pod.Spec.Containers[0].VolumeMounts
			require.Equal(t, 3, len(mounts), "expected 3 volume mounts in the application container")
			require.Equal(t, corev1.VolumeMount{
				Name:      volumeName,
				MountPath: "/opt/datadog-packages/datadog-apm-inject",
				SubPath:   "opt/datadog-packages/datadog-apm-inject",
			}, mounts[0], "expected first container volume mount to be the injector")
			require.Equal(t, corev1.VolumeMount{
				Name:      etcVolume.Name,
				MountPath: "/etc/ld.so.preload",
				SubPath:   "ld.so.preload",
				ReadOnly:  true,
			}, mounts[1], "expected first container volume mount to be the injector")
			require.Equal(t, corev1.VolumeMount{
				Name:      volumeName,
				MountPath: "/opt/datadog/apm/library",
				SubPath:   "opt/datadog/apm/library",
			}, mounts[2], "expected the second container volume mount to be the language libraries")

			if tt.expectedLdPreload != "" {
				requireEnv(t, "LD_PRELOAD", true, tt.expectedLdPreload)
			} else {
				requireEnv(t, "LD_PRELOAD", true, "/opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so")
			}

			requireEnv(t, "DD_INJECT_SENDER_TYPE", true, "k8s")
			requireEnv(t, "DD_INSTRUMENTATION_INSTALL_TYPE", true, tt.expectedInstallType)

			if tt.libInfo.languageDetection == nil {
				requireEnv(t, "DD_INSTRUMENTATION_LANGUAGES_DETECTED", false, "")
				requireEnv(t, "DD_INSTRUMENTATION_LANGUAGE_DETECTION_INJECTION_ENABLED", false, "")
			} else {
				requireEnv(t, "DD_INSTRUMENTATION_LANGUAGES_DETECTED", true, tt.expectedLangsDetected)
				requireEnv(t, "DD_INSTRUMENTATION_LANGUAGE_DETECTION_INJECTION_ENABLED", true, strconv.FormatBool(tt.libInfo.languageDetection.injectionEnabled))
			}

			for k, v := range tt.expectedLibConfigEnvs {
				requireEnv(t, k, true, v)
			}

			if len(tt.pod.Spec.Containers) > 1 {
				tt.assertExtraContainer(t, tt.pod.Spec.Containers[1])
			}
		})
	}
}

func TestMutatorCoreNewInjector(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("apm_config.instrumentation.version", "v2")
	wmeta := fxutil.Test[workloadmeta.Component](t,
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
	config, err := NewConfig(mockConfig)
	require.NoError(t, err)
	m, err := NewNamespaceMutator(config, wmeta)
	require.NoError(t, err)
	core := m.core

	// common vars
	startTime := time.Now()
	pod := &corev1.Pod{}

	i := core.newInjector(pod, startTime, libRequirementOptions{})
	require.Equal(t, &injector{
		injectTime: startTime,
		registry:   core.config.containerRegistry,
		image:      core.config.containerRegistry + "/apm-inject:0",
	}, i)

	core.config.Instrumentation.InjectorImageTag = "banana"
	i = core.newInjector(pod, startTime, libRequirementOptions{})
	require.Equal(t, &injector{
		injectTime: startTime,
		registry:   core.config.containerRegistry,
		image:      core.config.containerRegistry + "/apm-inject:banana",
	}, i)
}

func TestExtractLibInfo(t *testing.T) {
	// TODO: Add new entry when a new language is supported
	allLatestDefaultLibs := []libInfo{
		defaultLibInfo(java),
		defaultLibInfo(js),
		defaultLibInfo(python),
		defaultLibInfo(dotnet),
		defaultLibInfo(ruby),
		defaultLibInfo(php),
	}

	var mockConfig model.Config
	tests := []struct {
		name                   string
		pod                    *corev1.Pod
		deployments            []common.MockDeployment
		assertExtractedLibInfo func(*testing.T, extractedPodLibInfo)
		containerRegistry      string
		expectedLibsToInject   []libInfo
		expectedPodEligible    *bool
		setupConfig            func()
	}{
		{
			name:              "java",
			pod:               common.FakePodWithAnnotation("admission.datadoghq.com/java-lib.version", "v1"),
			containerRegistry: "registry",
			expectedLibsToInject: []libInfo{
				{
					lang:  "java",
					image: "registry/dd-lib-java-init:v1",
				},
			},
		},
		{
			name:              "java with default version",
			pod:               common.FakePodWithAnnotation("admission.datadoghq.com/java-lib.version", "default"),
			containerRegistry: "registry",
			expectedLibsToInject: []libInfo{
				{
					lang:  "java",
					image: "registry/dd-lib-java-init:v1",
				},
			},
		},
		{
			name:              "java from common registry",
			pod:               common.FakePodWithAnnotation("admission.datadoghq.com/java-lib.version", "v1"),
			containerRegistry: "",
			expectedLibsToInject: []libInfo{
				{
					lang:  "java",
					image: fmt.Sprintf("%s/dd-lib-java-init:v1", commonRegistry),
				},
			},
		},
		{
			name:              "js",
			pod:               common.FakePodWithAnnotation("admission.datadoghq.com/js-lib.version", "v1"),
			containerRegistry: "registry",
			expectedLibsToInject: []libInfo{
				{
					lang:  "js",
					image: "registry/dd-lib-js-init:v1",
				},
			},
		},
		{
			name:                "python",
			pod:                 common.FakePodWithAnnotation("admission.datadoghq.com/python-lib.version", "v1"),
			containerRegistry:   "registry",
			expectedPodEligible: pointer.Ptr(true),
			expectedLibsToInject: []libInfo{
				python.libInfo("", "registry/dd-lib-python-init:v1"),
			},
		},
		{
			name:                "python with unlabelled injection off",
			pod:                 common.FakePodWithAnnotation("admission.datadoghq.com/python-lib.version", "v1"),
			containerRegistry:   "registry",
			expectedPodEligible: pointer.Ptr(false),
			expectedLibsToInject: []libInfo{
				python.libInfo("", "registry/dd-lib-python-init:v1"),
			},
			setupConfig: func() {
				mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", false)
			},
		},
		{
			name:              "custom",
			pod:               common.FakePodWithAnnotation("admission.datadoghq.com/java-lib.custom-image", "custom/image"),
			containerRegistry: "registry",
			expectedLibsToInject: []libInfo{
				java.libInfo("", "custom/image"),
			},
		},
		{
			name:                 "unknown",
			pod:                  common.FakePodWithAnnotation("admission.datadoghq.com/unknown-lib.version", "v1"),
			containerRegistry:    "registry",
			expectedLibsToInject: []libInfo{},
		},
		{
			name: "java and js",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"admission.datadoghq.com/java-lib.version": "v1",
						"admission.datadoghq.com/js-lib.version":   "v1",
					},
				},
			},
			containerRegistry: "registry",
			expectedLibsToInject: []libInfo{
				{
					lang:  "java",
					image: "registry/dd-lib-java-init:v1",
				},
				{
					lang:  "js",
					image: "registry/dd-lib-js-init:v1",
				},
			},
		},
		{
			name: "java and js on specific containers",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"admission.datadoghq.com/java-app.java-lib.version": "v1",
						"admission.datadoghq.com/node-app.js-lib.version":   "v1",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "java-app",
						},
						{
							Name: "node-app",
						},
					},
				},
			},
			containerRegistry: "registry",
			expectedLibsToInject: []libInfo{
				{
					ctrName: "java-app",
					lang:    "java",
					image:   "registry/dd-lib-java-init:v1",
				},
				{
					ctrName: "node-app",
					lang:    "js",
					image:   "registry/dd-lib-js-init:v1",
				},
			},
		},
		{
			name:              "ruby",
			pod:               common.FakePodWithAnnotation("admission.datadoghq.com/ruby-lib.version", "v1"),
			containerRegistry: "registry",
			expectedLibsToInject: []libInfo{
				{
					lang:  "ruby",
					image: "registry/dd-lib-ruby-init:v1",
				},
			},
		},
		{
			name:                 "all",
			pod:                  common.FakePodWithAnnotation("admission.datadoghq.com/all-lib.version", "latest"),
			containerRegistry:    "registry",
			expectedPodEligible:  pointer.Ptr(true),
			expectedLibsToInject: allLatestDefaultLibs,
		},
		{
			name:                 "all with mutate_unlabelled off",
			pod:                  common.FakePodWithAnnotation("admission.datadoghq.com/all-lib.version", "latest"),
			containerRegistry:    "registry",
			expectedPodEligible:  pointer.Ptr(false),
			expectedLibsToInject: allLatestDefaultLibs,
			setupConfig: func() {
				mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", false)
			},
		},
		{
			name: "all with mutate_unlabelled off, but labelled admission enabled",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"admission.datadoghq.com/all-lib.version": "latest",
					},
					Labels: map[string]string{
						"admission.datadoghq.com/enabled": "true",
					},
				},
			},
			containerRegistry:    "registry",
			expectedPodEligible:  pointer.Ptr(true),
			expectedLibsToInject: allLatestDefaultLibs,
			setupConfig: func() {
				mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", false)
			},
		},
		{
			name:                 "all with mutate_unlabelled off",
			pod:                  common.FakePodWithAnnotation("admission.datadoghq.com/all-lib.version", "latest"),
			containerRegistry:    "registry",
			expectedPodEligible:  pointer.Ptr(false),
			expectedLibsToInject: allLatestDefaultLibs,
			setupConfig: func() {
				mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", false)
			},
		},
		{
			name: "all with mutate_unlabelled off, but labelled admission enabled",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"admission.datadoghq.com/all-lib.version": "latest",
					},
					Labels: map[string]string{
						"admission.datadoghq.com/enabled": "true",
					},
				},
			},
			containerRegistry:    "registry",
			expectedPodEligible:  pointer.Ptr(true),
			expectedLibsToInject: allLatestDefaultLibs,
			setupConfig: func() {
				mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", false)
			},
		},
		{
			name:                 "all with mutate_unlabelled off",
			pod:                  common.FakePodWithAnnotation("admission.datadoghq.com/all-lib.version", "latest"),
			containerRegistry:    "registry",
			expectedPodEligible:  pointer.Ptr(false),
			expectedLibsToInject: allLatestDefaultLibs,
			setupConfig: func() {
				mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", false)
			},
		},
		{
			name: "all with mutate_unlabelled off, but labelled admission enabled",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"admission.datadoghq.com/all-lib.version": "latest",
					},
					Labels: map[string]string{
						"admission.datadoghq.com/enabled": "true",
					},
				},
			},
			containerRegistry:    "registry",
			expectedPodEligible:  pointer.Ptr(true),
			expectedLibsToInject: allLatestDefaultLibs,
			setupConfig: func() {
				mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", false)
			},
		},
		{
			name: "java + all",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"admission.datadoghq.com/all-lib.version":  "latest",
						"admission.datadoghq.com/java-lib.version": "v1",
					},
				},
			},
			containerRegistry: "registry",
			expectedLibsToInject: []libInfo{
				{
					lang:  "java",
					image: "registry/dd-lib-java-init:v1",
				},
			},
		},
		{
			name:                 "all with unsupported version",
			pod:                  common.FakePodWithAnnotation("admission.datadoghq.com/all-lib.version", "unsupported"),
			containerRegistry:    "registry",
			expectedLibsToInject: allLatestDefaultLibs,
			setupConfig:          func() { mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", false) },
		},
		{
			name:                 "single step instrumentation with no pinned versions",
			pod:                  common.FakePodWithNamespaceAndLabel("ns", "", ""),
			containerRegistry:    "registry",
			expectedLibsToInject: allLatestDefaultLibs,
			setupConfig:          func() { mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true) },
		},
		{
			name:              "single step instrumentation with pinned java version",
			pod:               common.FakePodWithNamespaceAndLabel("ns", "", ""),
			containerRegistry: "registry",
			expectedLibsToInject: []libInfo{
				{
					lang:  "java",
					image: "registry/dd-lib-java-init:v1.20.0",
				},
			},
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true)
				mockConfig.SetWithoutSource("apm_config.instrumentation.lib_versions", map[string]string{"java": "v1.20.0"})
			},
		},
		{
			name:              "single step instrumentation with default java version",
			pod:               common.FakePodWithNamespaceAndLabel("ns", "", ""),
			containerRegistry: "registry",
			expectedLibsToInject: []libInfo{
				defaultLibInfo(java),
			},
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true)
				mockConfig.SetWithoutSource("apm_config.instrumentation.lib_versions", map[string]string{"java": "default"})
			},
		},
		{
			name:              "single step instrumentation with pinned java and python versions",
			pod:               common.FakePodWithNamespaceAndLabel("ns", "", ""),
			containerRegistry: "registry",
			expectedLibsToInject: []libInfo{
				{
					lang:  "java",
					image: "registry/dd-lib-java-init:v1.20.0",
				},
				{
					lang:  "python",
					image: "registry/dd-lib-python-init:v1.19.0",
				},
			},
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true)
				mockConfig.SetWithoutSource("apm_config.instrumentation.lib_versions", map[string]string{"java": "v1.20.0", "python": "v1.19.0"})
			},
		},
		{
			name:              "single step instrumentation with pinned java version and java annotation",
			pod:               common.FakePodWithAnnotation("admission.datadoghq.com/java-lib.version", "v1"),
			containerRegistry: "registry",
			expectedLibsToInject: []libInfo{
				{
					lang:  "java",
					image: "registry/dd-lib-java-init:v1",
				},
			},
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true)
				mockConfig.SetWithoutSource("apm_config.instrumentation.lib_versions", map[string]string{"java": "v1.20.0"})
				mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", true)
			},
		},
		{
			name: "pod with lang-detection deployment and default libs",
			pod: common.FakePodSpec{
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments: []common.MockDeployment{
				{
					ContainerName:  "pod",
					DeploymentName: "deployment",
					Namespace:      "ns",
					Languages:      languageSetOf("python"),
				},
			},
			containerRegistry: "registry",
			assertExtractedLibInfo: func(t *testing.T, i extractedPodLibInfo) {
				t.Helper()
				require.Equal(t, &libInfoLanguageDetection{
					libs: []libInfo{
						python.defaultLibInfo("registry", "pod"),
					},
					injectionEnabled: true,
				}, i.languageDetection)
			},
			expectedLibsToInject: []libInfo{
				python.defaultLibInfo("registry", "pod"),
			},
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true)
				mockConfig.SetWithoutSource("apm_config.instrumentation.lib_versions", defaultLibraries)
				mockConfig.SetWithoutSource("language_detection.enabled", true)
				mockConfig.SetWithoutSource("language_detection.reporting.enabled", true)
				mockConfig.SetWithoutSource("admission_controller.auto_instrumentation.inject_auto_detected_libraries", true)
			},
		},
		{
			name: "pod with lang-detection deployment and libs set",
			pod: common.FakePodSpec{
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments: []common.MockDeployment{
				{
					ContainerName:  "pod",
					DeploymentName: "deployment",
					Namespace:      "ns",
					Languages:      languageSetOf("python"),
				},
			},
			containerRegistry: "registry",
			assertExtractedLibInfo: func(t *testing.T, i extractedPodLibInfo) {
				t.Helper()
				require.Equal(t, &libInfoLanguageDetection{
					libs:             []libInfo{python.defaultLibInfo("registry", "pod")},
					injectionEnabled: true,
				}, i.languageDetection)
			},
			expectedLibsToInject: []libInfo{
				java.defaultLibInfo("registry", ""),
			},
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true)
				mockConfig.SetWithoutSource("apm_config.instrumentation.lib_versions", defaultLibrariesFor("java"))
				mockConfig.SetWithoutSource("language_detection.enabled", true)
				mockConfig.SetWithoutSource("language_detection.reporting.enabled", true)
				mockConfig.SetWithoutSource("admission_controller.auto_instrumentation.inject_auto_detected_libraries", true)
			},
		},
		{
			name:              "php",
			pod:               common.FakePodWithAnnotation("admission.datadoghq.com/php-lib.version", "v1"),
			containerRegistry: "registry",
			expectedLibsToInject: []libInfo{
				{
					lang:  "php",
					image: "registry/dd-lib-php-init:v1",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrides := map[string]interface{}{
				"admission_controller.mutate_unlabelled":  true,
				"admission_controller.container_registry": commonRegistry,
			}
			if tt.containerRegistry != "" {
				overrides["admission_controller.auto_instrumentation.container_registry"] = tt.containerRegistry
			}
			mockConfig = configmock.New(t)
			for k, v := range overrides {
				mockConfig.SetWithoutSource(k, v)
			}

			wmeta := common.FakeStoreWithDeployment(t, tt.deployments)
			mockConfig = configmock.New(t)
			for k, v := range overrides {
				mockConfig.SetWithoutSource(k, v)
			}

			if tt.setupConfig != nil {
				tt.setupConfig()
			}

			config, err := NewConfig(mockConfig)
			require.NoError(t, err)
			mutator, err := NewNamespaceMutator(config, wmeta)
			require.NoError(t, err)

			if tt.expectedPodEligible != nil {
				require.Equal(t, *tt.expectedPodEligible, mutator.isPodEligible(tt.pod))
			}

			extracted := mutator.extractLibInfo(tt.pod)
			if tt.assertExtractedLibInfo != nil {
				tt.assertExtractedLibInfo(t, extracted)
			}
			require.ElementsMatch(t, tt.expectedLibsToInject, extracted.libs)
		})
	}
}

func TestInjectLibConfig(t *testing.T) {
	tests := []struct {
		name         string
		pod          *corev1.Pod
		lang         language
		wantErr      bool
		expectedEnvs []corev1.EnvVar
	}{
		{
			name:    "nominal case",
			pod:     common.FakePodWithAnnotation("admission.datadoghq.com/java-lib.config.v1", `{"version":1,"service_language":"java","runtime_metrics_enabled":true,"tracing_rate_limit":50}`),
			lang:    java,
			wantErr: false,
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_RATE_LIMIT",
					Value: "50",
				},
			},
		},
		{
			name:    "inject all case",
			pod:     common.FakePodWithAnnotation("admission.datadoghq.com/all-lib.config.v1", `{"version":1,"service_language":"all","runtime_metrics_enabled":true,"tracing_rate_limit":50}`),
			lang:    "all",
			wantErr: false,
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_RATE_LIMIT",
					Value: "50",
				},
			},
		},
		{
			name:         "invalid json",
			pod:          common.FakePodWithAnnotation("admission.datadoghq.com/java-lib.config.v1", "invalid"),
			lang:         java,
			wantErr:      true,
			expectedEnvs: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := injectLibConfig(tt.pod, tt.lang)
			require.False(t, (err != nil) != tt.wantErr)
			if err != nil {
				return
			}
			container := tt.pod.Spec.Containers[0]
			envCount := 0
			for _, expectEnv := range tt.expectedEnvs {
				for _, contEnv := range container.Env {
					if expectEnv.Name == contEnv.Name {
						require.Equal(t, expectEnv.Value, contEnv.Value)
						envCount++
						break
					}
				}
			}
			require.Equal(t, len(tt.expectedEnvs), envCount)
		})
	}
}

func TestInjectLibInitContainer(t *testing.T) {
	tests := []struct {
		name                      string
		cpu                       string
		mem                       string
		pod                       *corev1.Pod
		image                     string
		lang                      language
		wantSkipInjection         bool
		resourceRequireAnnotation string
		wantErr                   bool
		wantCPU                   string
		wantMem                   string
		limitCPU                  string
		limitMem                  string
		secCtx                    *corev1.SecurityContext
	}{
		{
			name:    "no_resources,no_security_context",
			pod:     common.FakePod("java-pod"),
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "0",
			wantMem: "0",
			secCtx:  &corev1.SecurityContext{},
		},
		{
			name:    "with_resources",
			pod:     common.FakePod("java-pod"),
			cpu:     "100m",
			mem:     "500",
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "100m",
			wantMem: "500",
			secCtx:  &corev1.SecurityContext{},
		},
		{
			name:    "cpu_only",
			pod:     common.FakePod("java-pod"),
			cpu:     "200m",
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "200m",
			wantMem: "0",
			secCtx:  &corev1.SecurityContext{},
		},
		{
			name:    "memory_only",
			pod:     common.FakePod("java-pod"),
			mem:     "512Mi",
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "0",
			wantMem: "512Mi",
			secCtx:  &corev1.SecurityContext{},
		},
		{
			name:    "with_invalid_resources",
			pod:     common.FakePod("java-pod"),
			cpu:     "foo",
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: true,
			wantCPU: "0",
			wantMem: "0",
			secCtx:  &corev1.SecurityContext{},
		},
		{
			name:    "with_full_security_context",
			pod:     common.FakePod("java-pod"),
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "0",
			wantMem: "0",
			secCtx: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Add:  []corev1.Capability{"NET_ADMIN", "SYS_TIME"},
					Drop: []corev1.Capability{"ALL"},
				},
				Privileged: pointer.Ptr(false),
				SELinuxOptions: &corev1.SELinuxOptions{
					User:  "test",
					Role:  "root",
					Type:  "none",
					Level: "s0:c123,c456",
				},
				WindowsOptions: &corev1.WindowsSecurityContextOptions{
					GMSACredentialSpecName: pointer.Ptr("Developer"),
					GMSACredentialSpec:     pointer.Ptr("http://localhost:8081"),
					RunAsUserName:          pointer.Ptr("Developer"),
					HostProcess:            pointer.Ptr(false),
				},
				RunAsUser:                pointer.Ptr(int64(1001)),
				RunAsGroup:               pointer.Ptr(int64(5)),
				RunAsNonRoot:             pointer.Ptr(true),
				ReadOnlyRootFilesystem:   pointer.Ptr(true),
				AllowPrivilegeEscalation: pointer.Ptr(false),
				ProcMount:                pointer.Ptr(corev1.DefaultProcMount),
				SeccompProfile: &corev1.SeccompProfile{
					Type:             "LocalHost",
					LocalhostProfile: pointer.Ptr("my-profiles/profile-allow.json"),
				},
			},
		},
		{
			name:    "with_limited_security_context",
			pod:     common.FakePod("java-pod"),
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "0",
			wantMem: "0",
			secCtx: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
				RunAsNonRoot:             pointer.Ptr(true),
				ReadOnlyRootFilesystem:   pointer.Ptr(true),
				AllowPrivilegeEscalation: pointer.Ptr(false),
				SeccompProfile: &corev1.SeccompProfile{
					Type: "RuntimeDefault",
				},
			},
		},
		{
			name: "with_container_resources",
			pod: common.FakePodWithResources("java-pod", corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("499m"),
					corev1.ResourceMemory: resource.MustParse("101Mi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("499m"),
					corev1.ResourceMemory: resource.MustParse("101Mi"),
				},
			}),
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "499m",
			wantMem: "101Mi",
		},
		{
			name: "init_container_resources",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "java-pod",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{Name: "with_init_container_resources_init-1", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("499m"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("499m"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
					}}, {Name: "with_init_container_resources_init-2", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("501m"),
							corev1.ResourceMemory: resource.MustParse("99Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("501m"),
							corev1.ResourceMemory: resource.MustParse("99Mi"),
						},
					}}},
					Containers: []corev1.Container{{Name: "c1"}},
				},
			},
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "501m",
			wantMem: "101Mi",
		},
		{
			name: "multiple_container_resources",
			pod: common.FakePodWithContainer("java-pod", corev1.Container{
				Name: "c1",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("499m"),
						corev1.ResourceMemory: resource.MustParse("101Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("499m"),
						corev1.ResourceMemory: resource.MustParse("101Mi"),
					},
				},
			}, corev1.Container{
				Name: "c2",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("499m"),
						corev1.ResourceMemory: resource.MustParse("101Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("499m"),
						corev1.ResourceMemory: resource.MustParse("101Mi"),
					},
				},
			}),
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "998m",
			wantMem: "202Mi",
		},
		{
			name: "container_and_init_container",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "java-pod",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{Name: "i1", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("501m"),
							corev1.ResourceMemory: resource.MustParse("99Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("501m"),
							corev1.ResourceMemory: resource.MustParse("99Mi"),
						},
					}}},
					Containers: []corev1.Container{{Name: "c1", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("499m"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("499m"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
					}}},
				},
			},
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "501m",
			wantMem: "101Mi",
		},
		{
			name: "config_and_resources",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "java-pod",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{Name: "i1", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("501m"),
							corev1.ResourceMemory: resource.MustParse("99Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("501m"),
							corev1.ResourceMemory: resource.MustParse("99Mi"),
						},
					}}},
					Containers: []corev1.Container{{Name: "c1", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("499m"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("499m"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
					}}},
				},
			},
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "501m",
			wantMem: "101Mi",
		},
		{
			name: "config_and_resources",
			pod: common.FakePodWithContainer("java-pod", corev1.Container{Name: "c1", Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("499m"),
					corev1.ResourceMemory: resource.MustParse("101Mi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("499m"),
					corev1.ResourceMemory: resource.MustParse("101Mi"),
				},
			}}),
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			cpu:     "100m",
			mem:     "256Mi",
			wantErr: false,
			wantCPU: "100m",
			wantMem: "256Mi",
		},
		{
			name: "low_memory_skip",
			pod: common.FakePodWithContainer("java-pod", corev1.Container{Name: "c1", Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("499m"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("499m"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
			}}),
			image:                     "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:                      java,
			wantErr:                   false,
			wantSkipInjection:         true,
			resourceRequireAnnotation: "The overall pod's containers limit is too low, memory pod_limit=50Mi needed=100Mi",
		},
		{
			name: "low_cpu_skip",
			pod: common.FakePodWithContainer("java-pod", corev1.Container{Name: "c1", Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("0.025"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("0.025"),
				},
			}}),
			image:                     "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:                      java,
			wantErr:                   false,
			wantSkipInjection:         true,
			resourceRequireAnnotation: "The overall pod's containers limit is too low, cpu pod_limit=25m needed=50m",
		},
		{
			name: "both_cpu_memory_skip",
			pod: common.FakePodWithContainer("java-pod", corev1.Container{Name: "c1", Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("0.025"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("0.025"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
			}}),
			image:                     "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:                      java,
			wantErr:                   false,
			wantSkipInjection:         true,
			resourceRequireAnnotation: "The overall pod's containers limit is too low, cpu pod_limit=25m needed=50m, memory pod_limit=50Mi needed=100Mi",
		},
		{
			name: "config_override_low_limit_skip",
			pod: common.FakePodWithContainer("java-pod", corev1.Container{Name: "c1", Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("499m"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("499m"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
			}}),
			image:             "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:              java,
			cpu:               "100",
			mem:               "51Mi",
			wantErr:           false,
			wantSkipInjection: false,
			wantCPU:           "100",
			wantMem:           "51Mi",
		},
		{
			name: "sidecar_container",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "java-pod",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name: "init-container-1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("501m"),
									corev1.ResourceMemory: resource.MustParse("101Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("501m"),
									corev1.ResourceMemory: resource.MustParse("101Mi"),
								},
							},
						}, {
							Name:          "sidecar-container-1",
							RestartPolicy: pointer.Ptr(corev1.ContainerRestartPolicyAlways),
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("50Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("50Mi"),
								},
							},
						},
					},
					Containers: []corev1.Container{{Name: "c1", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
					}}},
				},
			},
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "700m",
			wantMem: "151Mi",
		},
		{
			name: "init_container_request_greater_than_limit",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "java-pod",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{Name: "i1", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{}, // No limits
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("200Mi"),
						},
					}}},
					Containers: []corev1.Container{{Name: "c1", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
					}}},
				},
			},
			image:    "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:     java,
			wantErr:  false,
			wantCPU:  "200m",
			wantMem:  "200Mi",
			limitCPU: "200m",
			limitMem: "200Mi",
		},
		{
			name: "containers_request_greater_than_limit",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "java-pod",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{Name: "i1", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
					}}},
					Containers: []corev1.Container{{Name: "c1", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{}, // No limits
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("200Mi"),
						},
					}}},
				},
			},
			image:    "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:     java,
			wantErr:  false,
			wantCPU:  "200m",
			wantMem:  "200Mi",
			limitCPU: "200m",
			limitMem: "200Mi",
		},
		{
			name: "sidecar_container_request_greater_than_limit",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "java-pod",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name: "init-container-1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("501m"),
									corev1.ResourceMemory: resource.MustParse("101Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("501m"),
									corev1.ResourceMemory: resource.MustParse("101Mi"),
								},
							},
						}, {
							Name:          "sidecar-container-1",
							RestartPolicy: pointer.Ptr(corev1.ContainerRestartPolicyAlways),
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("50Mi"),
								},
							},
						},
					},
					Containers: []corev1.Container{{Name: "c1", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
					}}},
				},
			},
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "700m",
			wantMem: "151Mi",
		},
		{
			name: "todo",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "java-pod",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "1", Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("20m"),
								corev1.ResourceMemory: resource.MustParse("50Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("20m"),
								corev1.ResourceMemory: resource.MustParse("50Mi"),
							},
						}},
					},
					Containers: []corev1.Container{
						{Name: "c1", Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1m"),
								corev1.ResourceMemory: resource.MustParse("8Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1m"),
								corev1.ResourceMemory: resource.MustParse("8Mi"),
							},
						}},
						{Name: "c1", Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("2"),
								corev1.ResourceMemory: resource.MustParse("8692Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("2"),
								corev1.ResourceMemory: resource.MustParse("8692Mi"),
							},
						}},
						{Name: "c2", Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
						}},
					},
				},
			},
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "2011m",
			wantMem: "8764Mi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wmeta := fxutil.Test[workloadmeta.Component](t,
				core.MockBundle(),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			)

			mockConfig := configmock.New(t)
			if tt.cpu != "" {
				mockConfig.SetWithoutSource("admission_controller.auto_instrumentation.init_resources.cpu", tt.cpu)
			}
			if tt.mem != "" {
				mockConfig.SetWithoutSource("admission_controller.auto_instrumentation.init_resources.memory", tt.mem)
			}

			config, err := NewConfig(mockConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("injectLibInitContainer() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			// N.B. this is a bit hacky but consistent.
			config.initSecurityContext = tt.secCtx

			mutator, err := NewNamespaceMutator(config, wmeta)
			require.NoError(t, err)

			c := tt.lang.libInfo("", tt.image).initContainers(config.version)[0]
			requirements, injectionDecision := initContainerResourceRequirements(tt.pod, config.defaultResourceRequirements)
			require.Equal(t, tt.wantSkipInjection, injectionDecision.skipInjection)
			require.Equal(t, tt.resourceRequireAnnotation, injectionDecision.message)
			if tt.wantSkipInjection {
				return
			}

			c.Mutators = mutator.core.newInitContainerMutators(requirements, tt.pod.Namespace)
			initalInitContainerCount := len(tt.pod.Spec.InitContainers)
			err = c.mutatePod(tt.pod)
			if (err != nil) != tt.wantErr {
				t.Errorf("injectLibInitContainer() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				return
			}
			require.Len(t, tt.pod.Spec.InitContainers, initalInitContainerCount+1)

			req := tt.pod.Spec.InitContainers[initalInitContainerCount].Resources.Requests[corev1.ResourceCPU]
			lim := tt.pod.Spec.InitContainers[initalInitContainerCount].Resources.Limits[corev1.ResourceCPU]
			requestCPUQuantity := resource.MustParse(tt.wantCPU)
			limitCPUQuantity := requestCPUQuantity
			if tt.limitCPU != "" {
				limitCPUQuantity = resource.MustParse(tt.limitCPU)
			}

			t.Log("expected CPU request/limit:", requestCPUQuantity.String(), "/", limitCPUQuantity.String(), ", actual request/limit:", req.String(), "/", lim.String())
			require.Zero(t, requestCPUQuantity.Cmp(req), "expected CPU request: %s, actual: %s", requestCPUQuantity.String(), req.String()) // Cmp returns 0 if equal
			require.Zero(t, limitCPUQuantity.Cmp(lim), "expected CPU limit: %s, actual: %s", limitCPUQuantity.String(), lim.String())

			req = tt.pod.Spec.InitContainers[initalInitContainerCount].Resources.Requests[corev1.ResourceMemory]
			lim = tt.pod.Spec.InitContainers[initalInitContainerCount].Resources.Limits[corev1.ResourceMemory]
			requestMemQuantity := resource.MustParse(tt.wantMem)
			limitMemQuantity := requestMemQuantity
			if tt.limitMem != "" {
				limitMemQuantity = resource.MustParse(tt.limitMem)
			}

			t.Log("expected memory request/limit:", requestMemQuantity.String(), "/", limitMemQuantity.String(), ", actual request/limit:", req.String(), "/", lim.String())
			require.Zero(t, requestMemQuantity.Cmp(req), "expected memory request: %s, actual: %s", requestMemQuantity.String(), req.String())
			require.Zero(t, limitMemQuantity.Cmp(lim), "expected memory limit: %s, actual: %s", limitMemQuantity.String(), lim.String())

			expSecCtx := tt.pod.Spec.InitContainers[0].SecurityContext
			require.Equal(t, tt.secCtx, expSecCtx)
		})
	}
}

func expBasicConfig() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "DD_RUNTIME_METRICS_ENABLED",
			Value: "true",
		},
		{
			Name:  "DD_TRACE_HEALTH_METRICS_ENABLED",
			Value: "true",
		},
		{
			Name:  "DD_TRACE_ENABLED",
			Value: "true",
		},
		{
			Name:  "DD_LOGS_INJECTION",
			Value: "true",
		},
	}
}

func injectAllEnvs() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "PYTHONPATH",
			Value: "/datadog-lib/",
		},
		{
			Name:  "RUBYOPT",
			Value: " -r/datadog-lib/auto_inject",
		},
		{
			Name:  "NODE_OPTIONS",
			Value: " --require=/datadog-lib/node_modules/dd-trace/init",
		},
		{
			Name:  "JAVA_TOOL_OPTIONS",
			Value: " -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log",
		},
		{
			Name:  "DD_DOTNET_TRACER_HOME",
			Value: "/datadog-lib",
		},
		{
			Name:  "CORECLR_ENABLE_PROFILING",
			Value: "1",
		},
		{
			Name:  "CORECLR_PROFILER",
			Value: "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}",
		},
		{
			Name:  "CORECLR_PROFILER_PATH",
			Value: "/datadog-lib/Datadog.Trace.ClrProfiler.Native.so",
		},
		{
			Name:  "DD_TRACE_LOG_DIRECTORY",
			Value: "/datadog-lib/logs",
		},
	}
}

func TestInjectAutoInstrumentationV1(t *testing.T) {
	var (
		mockConfig model.Config
		wConfig    = func(k string, v any) func() {
			return func() {
				mockConfig.SetWithoutSource(k, v)
			}
		}

		enableAPMInstrumentation  = wConfig("apm_config.instrumentation.enabled", true)
		disableAPMInstrumentation = wConfig("apm_config.instrumentation.enabled", false)
		disableNamespaces         = func(ns ...string) func() {
			return wConfig("apm_config.instrumentation.disabled_namespaces", ns)
		}
		enabledNamespaces = func(ns ...string) func() {
			return wConfig("apm_config.instrumentation.enabled_namespaces", ns)
		}
		withLibVersions = func(vs map[string]string) func() {
			return wConfig("apm_config.instrumentation.lib_versions", vs)
		}
		withInitSecurityConfig = func(sc string) func() {
			return wConfig("admission_controller.auto_instrumentation.init_security_context", sc)
		}
	)

	type funcs = []func()

	uuid := uuid.New().String()
	installTime := strconv.FormatInt(time.Now().Unix(), 10)

	tests := []struct {
		name                      string
		pod                       *corev1.Pod
		expectedEnvs              []corev1.EnvVar
		expectedInjectedLibraries map[string]string
		expectedSecurityContext   *corev1.SecurityContext
		langDetectionDeployments  []common.MockDeployment
		wantErr                   bool
		wantWebhookInitErr        bool
		setupConfig               funcs
	}{
		{
			name: "inject all with dotnet-profiler no service name when SSI disabled",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.version":   "latest",
					"admission.datadoghq.com/all-lib.config.v1": `{"version":1,"runtime_metrics_enabled":true,"tracing_rate_limit":50,"tracing_sampling_rate":0.3}`,
				},
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "true",
				},
				ParentKind: "replicaset",
				ParentName: "deployment-1234",
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_lib_injection",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_RATE_LIMIT",
					Value: "50",
				},
				{
					Name:  "DD_TRACE_SAMPLE_RATE",
					Value: "0.30",
				},
				{
					Name:  "PYTHONPATH",
					Value: "/datadog-lib/",
				},
				{
					Name:  "RUBYOPT",
					Value: " -r/datadog-lib/auto_inject",
				},
				{
					Name:  "NODE_OPTIONS",
					Value: " --require=/datadog-lib/node_modules/dd-trace/init",
				},
				{
					Name:  "JAVA_TOOL_OPTIONS",
					Value: " -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log",
				},
				{
					Name:  "DD_DOTNET_TRACER_HOME",
					Value: "/datadog-lib",
				},
				{
					Name:  "CORECLR_ENABLE_PROFILING",
					Value: "1",
				},
				{
					Name:  "CORECLR_PROFILER",
					Value: "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}",
				},
				{
					Name:  "CORECLR_PROFILER_PATH",
					Value: "/datadog-lib/Datadog.Trace.ClrProfiler.Native.so",
				},
				{
					Name:  "DD_TRACE_LOG_DIRECTORY",
					Value: "/datadog-lib/logs",
				},
			},
			expectedInjectedLibraries: defaultLibraries,
			wantErr:                   false,
			wantWebhookInitErr:        false,
		},
		{
			name: "inject all",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.version":   "latest",
					"admission.datadoghq.com/all-lib.config.v1": `{"version":1,"runtime_metrics_enabled":true,"tracing_rate_limit":50,"tracing_sampling_rate":0.3}`,
				},
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "true",
				},
				ParentName: "replicaset",
				ParentKind: "deployment-1234",
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_lib_injection",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_RATE_LIMIT",
					Value: "50",
				},
				{
					Name:  "DD_TRACE_SAMPLE_RATE",
					Value: "0.30",
				},
				{
					Name:  "PYTHONPATH",
					Value: "/datadog-lib/",
				},
				{
					Name:  "RUBYOPT",
					Value: " -r/datadog-lib/auto_inject",
				},
				{
					Name:  "NODE_OPTIONS",
					Value: " --require=/datadog-lib/node_modules/dd-trace/init",
				},
				{
					Name:  "JAVA_TOOL_OPTIONS",
					Value: " -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log",
				},
				{
					Name:  "DD_DOTNET_TRACER_HOME",
					Value: "/datadog-lib",
				},
				{
					Name:  "CORECLR_ENABLE_PROFILING",
					Value: "1",
				},
				{
					Name:  "CORECLR_PROFILER",
					Value: "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}",
				},
				{
					Name:  "CORECLR_PROFILER_PATH",
					Value: "/datadog-lib/Datadog.Trace.ClrProfiler.Native.so",
				},
				{
					Name:  "DD_TRACE_LOG_DIRECTORY",
					Value: "/datadog-lib/logs",
				},
			},
			expectedInjectedLibraries: defaultLibraries,
			wantErr:                   false,
			wantWebhookInitErr:        false,
		},
		{
			name: "inject library and all",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.version":   "latest",
					"admission.datadoghq.com/all-lib.config.v1": `{"version":1,"runtime_metrics_enabled":true,"tracing_rate_limit":50,"tracing_sampling_rate":0.3}`,
					"admission.datadoghq.com/js-lib.version":    "v1.10",
					"admission.datadoghq.com/js-lib.config.v1":  `{"version":1,"tracing_sampling_rate":0.4}`,
				},
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "true",
				},
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_RATE_LIMIT",
					Value: "50",
				},
				{
					Name:  "DD_TRACE_SAMPLE_RATE",
					Value: "0.40",
				},
				{
					Name:  "NODE_OPTIONS",
					Value: " --require=/datadog-lib/node_modules/dd-trace/init",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_lib_injection",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
			},
			expectedInjectedLibraries: map[string]string{"js": "v1.10"},
			wantErr:                   false,
			wantWebhookInitErr:        false,
		},
		{
			name: "inject library and all no library version",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.version":   "latest",
					"admission.datadoghq.com/all-lib.config.v1": `{"version":1,"runtime_metrics_enabled":true,"tracing_rate_limit":50,"tracing_sampling_rate":0.3}`,
					"admission.datadoghq.com/js-lib.config.v1":  `{"version":1,"tracing_sampling_rate":0.4}`,
				},
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "true",
				},
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_lib_injection",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_RATE_LIMIT",
					Value: "50",
				},
				{
					Name:  "PYTHONPATH",
					Value: "/datadog-lib/",
				},
				{
					Name:  "RUBYOPT",
					Value: " -r/datadog-lib/auto_inject",
				},
				{
					Name:  "NODE_OPTIONS",
					Value: " --require=/datadog-lib/node_modules/dd-trace/init",
				},
				{
					Name:  "JAVA_TOOL_OPTIONS",
					Value: " -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log",
				},
				{
					Name:  "DD_DOTNET_TRACER_HOME",
					Value: "/datadog-lib",
				},
				{
					Name:  "CORECLR_ENABLE_PROFILING",
					Value: "1",
				},
				{
					Name:  "CORECLR_PROFILER",
					Value: "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}",
				},
				{
					Name:  "CORECLR_PROFILER_PATH",
					Value: "/datadog-lib/Datadog.Trace.ClrProfiler.Native.so",
				},
				{
					Name:  "DD_TRACE_LOG_DIRECTORY",
					Value: "/datadog-lib/logs",
				},
				{
					Name:  "DD_TRACE_SAMPLE_RATE",
					Value: "0.40",
				},
			},
			expectedInjectedLibraries: defaultLibraries,
			wantErr:                   false,
			wantWebhookInitErr:        false,
		},
		{
			name: "inject all error - bad json",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					// TODO: we might not want to be injecting the libraries if the config is malformed
					"admission.datadoghq.com/all-lib.version":   "latest",
					"admission.datadoghq.com/all-lib.config.v1": `{"version":1,"runtime_metrics_enabled":true,`,
				},
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "true",
				},
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_lib_injection",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
				{
					Name:  "PYTHONPATH",
					Value: "/datadog-lib/",
				},
				{
					Name:  "RUBYOPT",
					Value: " -r/datadog-lib/auto_inject",
				},
				{
					Name:  "NODE_OPTIONS",
					Value: " --require=/datadog-lib/node_modules/dd-trace/init",
				},
				{
					Name:  "JAVA_TOOL_OPTIONS",
					Value: " -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log",
				},
				{
					Name:  "DD_DOTNET_TRACER_HOME",
					Value: "/datadog-lib",
				},
				{
					Name:  "CORECLR_ENABLE_PROFILING",
					Value: "1",
				},
				{
					Name:  "CORECLR_PROFILER",
					Value: "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}",
				},
				{
					Name:  "CORECLR_PROFILER_PATH",
					Value: "/datadog-lib/Datadog.Trace.ClrProfiler.Native.so",
				},
				{
					Name:  "DD_TRACE_LOG_DIRECTORY",
					Value: "/datadog-lib/logs",
				},
			},
			expectedInjectedLibraries: defaultLibraries,
			wantErr:                   true,
			wantWebhookInitErr:        false,
		},
		{
			name: "inject java",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version":   "latest",
					"admission.datadoghq.com/java-lib.config.v1": `{"version":1,"tracing_sampling_rate":0.3}`,
				},
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "true",
				},
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_lib_injection",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
				{
					Name:  "DD_TRACE_SAMPLE_RATE",
					Value: "0.30",
				},
				{
					Name:  "JAVA_TOOL_OPTIONS",
					Value: " -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log",
				},
			},
			expectedInjectedLibraries: map[string]string{"java": "latest"},
			wantErr:                   false,
			wantWebhookInitErr:        false,
		},
		{
			name: "inject python",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/python-lib.version":   "latest",
					"admission.datadoghq.com/python-lib.config.v1": `{"version":1,"tracing_sampling_rate":0.3}`,
				},
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "true",
				},
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_TRACE_SAMPLE_RATE",
					Value: "0.30",
				},
				{
					Name:  "PYTHONPATH",
					Value: "/datadog-lib/",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_lib_injection",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
			},
			expectedInjectedLibraries: map[string]string{"python": "latest"},
			wantErr:                   false,
			wantWebhookInitErr:        false,
		},
		{
			name: "inject node",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/js-lib.version":   "latest",
					"admission.datadoghq.com/js-lib.config.v1": `{"version":1,"tracing_sampling_rate":0.3}`,
				},
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "true",
				},
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_TRACE_SAMPLE_RATE",
					Value: "0.30",
				},
				{
					Name:  "NODE_OPTIONS",
					Value: " --require=/datadog-lib/node_modules/dd-trace/init",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_lib_injection",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
			},
			expectedInjectedLibraries: map[string]string{"js": "latest"},
			wantErr:                   false,
			wantWebhookInitErr:        false,
		},
		{
			name: "inject java bad json",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version":   "latest",
					"admission.datadoghq.com/java-lib.config.v1": `{"version":1,"runtime_metrics_enabled":true,`,
				},
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "true",
				},
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_lib_injection",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
				{
					Name:  "JAVA_TOOL_OPTIONS",
					Value: " -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log",
				},
			},
			expectedInjectedLibraries: map[string]string{"java": "latest"},
			wantErr:                   true,
			wantWebhookInitErr:        false,
		},
		{
			name: "inject with enabled false",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version":   "latest",
					"admission.datadoghq.com/java-lib.config.v1": `{"version":1,"runtime_metrics_enabled":true,`,
				},
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "false",
				},
			}.Create(),
			expectedEnvs:              nil,
			expectedInjectedLibraries: map[string]string{},
			wantErr:                   false,
			wantWebhookInitErr:        false,
		},
		{
			name: "Single Step Instrumentation: user configuration is respected",
			pod: common.FakePodSpec{
				Envs: []corev1.EnvVar{
					{
						Name:  "DD_SERVICE",
						Value: "user-deployment",
					},
					{
						Name:  "DD_TRACE_ENABLED",
						Value: "false",
					},
					{
						Name:  "DD_RUNTIME_METRICS_ENABLED",
						Value: "false",
					},
					{
						Name:  "DD_TRACE_HEALTH_METRICS_ENABLED",
						Value: "false",
					},
					{
						Name:  "DD_TRACE_SAMPLE_RATE",
						Value: "0.5",
					},
					{
						Name:  "DD_TRACE_RATE_LIMIT",
						Value: "2",
					},
					{
						Name:  "DD_LOGS_INJECTION",
						Value: "false",
					},
				},
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			expectedEnvs: append(injectAllEnvs(), []corev1.EnvVar{
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_single_step",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
				{
					Name:  "DD_SERVICE",
					Value: "user-deployment",
				},
				{
					Name:  "DD_TRACE_ENABLED",
					Value: "false",
				},
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "false",
				},
				{
					Name:  "DD_TRACE_HEALTH_METRICS_ENABLED",
					Value: "false",
				},
				{
					Name:  "DD_TRACE_SAMPLE_RATE",
					Value: "0.5",
				},
				{
					Name:  "DD_TRACE_RATE_LIMIT",
					Value: "2",
				},
				{
					Name:  "DD_LOGS_INJECTION",
					Value: "false",
				},
			}...),
			expectedInjectedLibraries: defaultLibraries,
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig:               funcs{enableAPMInstrumentation},
		},
		{
			name: "Single Step Instrumentation: disable with label",
			pod: common.FakePodSpec{
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "false",
				},
				ParentKind: "replicaset",
				ParentName: "test-deployment-123",
			}.Create(),
			expectedEnvs:              nil,
			expectedInjectedLibraries: map[string]string{},
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig:               funcs{enableAPMInstrumentation},
		},
		{
			name: "Single Step Instrumentation: default service name for ReplicaSet",
			pod: common.FakePodSpec{
				ParentKind: "replicaset",
				ParentName: "test-deployment-123",
			}.Create(),
			expectedEnvs: append(append(injectAllEnvs(), expBasicConfig()...),
				corev1.EnvVar{
					Name:  "DD_SERVICE",
					Value: "test-deployment",
				},
				corev1.EnvVar{
					Name:  "DD_SERVICE_K8S_ENV_SOURCE",
					Value: "owner=test-deployment",
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_single_step",
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
			),
			expectedInjectedLibraries: defaultLibraries,
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig:               funcs{enableAPMInstrumentation},
		},
		{
			name: "Single Step Instrumentation: default service name for StatefulSet",
			pod: common.FakePodSpec{
				ParentKind: "statefulset",
				ParentName: "test-statefulset-123",
			}.Create(),
			expectedEnvs: append(append(injectAllEnvs(), expBasicConfig()...),
				corev1.EnvVar{
					Name:  "DD_SERVICE",
					Value: "test-statefulset-123",
				},
				corev1.EnvVar{
					Name:  "DD_SERVICE_K8S_ENV_SOURCE",
					Value: "owner=test-statefulset-123",
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_single_step",
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
			),
			expectedInjectedLibraries: defaultLibraries,
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig:               funcs{enableAPMInstrumentation},
		},
		{
			name: "Single Step Instrumentation: default service name (disabled)",
			pod: common.FakePodSpec{
				ParentKind: "replicaset",
				ParentName: "test-deployment-123",
			}.Create(),
			expectedEnvs:              nil,
			expectedInjectedLibraries: map[string]string{},
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig:               funcs{disableAPMInstrumentation},
		},
		{
			name: "Single Step Instrumentation: disabled namespaces should not be instrumented",
			pod: common.FakePodSpec{
				ParentKind: "replicaset",
				ParentName: "test-deployment-123",
			}.Create(),
			expectedEnvs:              nil,
			expectedInjectedLibraries: map[string]string{},
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig:               funcs{enableAPMInstrumentation, disableNamespaces("ns")},
		},
		{
			name: "Single Step Instrumentation: enabled namespaces should be instrumented",
			pod: common.FakePodSpec{
				Envs: []corev1.EnvVar{
					{
						Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
						Value: "k8s_single_step",
					},
				},
				ParentKind: "replicaset",
				ParentName: "test-app-123",
			}.Create(),
			expectedEnvs: append(append(injectAllEnvs(), expBasicConfig()...),
				corev1.EnvVar{
					Name:  "DD_SERVICE",
					Value: "test-app",
				},
				corev1.EnvVar{
					Name:  "DD_SERVICE_K8S_ENV_SOURCE",
					Value: "owner=test-app",
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_single_step",
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
			),
			expectedInjectedLibraries: defaultLibraries,
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig:               funcs{enableAPMInstrumentation, enabledNamespaces("ns")},
		},
		{
			name: "Single Step Instrumentation enabled and language annotation provided",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/js-lib.version":   "v1.10",
					"admission.datadoghq.com/js-lib.config.v1": `{"version":1,"tracing_sampling_rate":0.4}`,
				},
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_SAMPLE_RATE",
					Value: "0.40",
				},
				{
					Name:  "DD_TRACE_HEALTH_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_LOGS_INJECTION",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_ENABLED",
					Value: "true",
				},
				{
					Name:  "NODE_OPTIONS",
					Value: " --require=/datadog-lib/node_modules/dd-trace/init",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_single_step",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
			},
			expectedInjectedLibraries: map[string]string{"js": "v1.10"},
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig:               funcs{enableAPMInstrumentation},
		},
		{
			name: "Single Step Instrumentation enabled with libVersions set",
			pod:  common.FakePodSpec{}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_HEALTH_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_LOGS_INJECTION",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_ENABLED",
					Value: "true",
				},
				{
					Name:  "JAVA_TOOL_OPTIONS",
					Value: " -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log",
				},
				{
					Name:  "PYTHONPATH",
					Value: "/datadog-lib/",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_single_step",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
			},
			expectedInjectedLibraries: map[string]string{"java": "v1.28.0", "python": "v3.6.0"},
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig: funcs{
				enableAPMInstrumentation,
				withLibVersions(map[string]string{"java": "v1.28.0", "python": "v3.6.0"}),
			},
		},
		{
			name: "Single Step Instrumentation enabled, with language annotation and libVersions set",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/js-lib.version": "v1.10",
				},
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_HEALTH_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_LOGS_INJECTION",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_ENABLED",
					Value: "true",
				},
				{
					Name:  "NODE_OPTIONS",
					Value: " --require=/datadog-lib/node_modules/dd-trace/init",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_single_step",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
			},
			expectedInjectedLibraries: map[string]string{"js": "v1.10"},
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig: funcs{
				enableAPMInstrumentation,
				withLibVersions(map[string]string{"java": "v1.28.0", "python": "v3.6.0"}),
			},
		},
		{
			name: "Single Step Instrumentation enabled and language detection",
			pod: common.FakePodSpec{
				ParentKind: "replicaset",
				ParentName: "test-app-689695b6cc",
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_SERVICE",
					Value: "test-app",
				},
				{
					Name:  "DD_SERVICE_K8S_ENV_SOURCE",
					Value: "owner=test-app",
				},
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_HEALTH_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_LOGS_INJECTION",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_ENABLED",
					Value: "true",
				},
				{
					Name:  "JAVA_TOOL_OPTIONS",
					Value: " -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log",
				},
				{
					Name:  "PYTHONPATH",
					Value: "/datadog-lib/",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_single_step",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
			},
			expectedInjectedLibraries: defaultLibrariesFor("python", "java"),
			langDetectionDeployments: []common.MockDeployment{
				{
					ContainerName:  "pod",
					DeploymentName: "test-app",
					Namespace:      "ns",
					Languages:      languageSetOf("python", "java"),
				},
			},
			wantErr:            false,
			wantWebhookInitErr: false,
			setupConfig: funcs{
				wConfig("admission_controller.auto_instrumentation.inject_auto_detected_libraries", true),
				wConfig("language_detection.enabled", true),
				wConfig("language_detection.reporting.enabled", true),
				enableAPMInstrumentation,
			},
		},
		{
			name: "Library annotation, Single Step Instrumentation with library pinned and language detection",
			pod: common.FakePodSpec{
				Annotations: map[string]string{"admission.datadoghq.com/js-lib.version": "v1.10"},
				ParentKind:  "replicaset",
				ParentName:  "test-app-689695b6cc",
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_SERVICE",
					Value: "test-app",
				},
				{
					Name:  "DD_SERVICE_K8S_ENV_SOURCE",
					Value: "owner=test-app",
				},
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_HEALTH_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_LOGS_INJECTION",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_ENABLED",
					Value: "true",
				},
				{
					Name:  "NODE_OPTIONS",
					Value: " --require=/datadog-lib/node_modules/dd-trace/init",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_single_step",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
			},
			expectedInjectedLibraries: map[string]string{"js": "v1.10"},
			langDetectionDeployments: []common.MockDeployment{
				{
					ContainerName:  "pod",
					DeploymentName: "test-app",
					Namespace:      "ns",
					Languages:      languageSetOf("python", "java"),
				},
			},
			wantErr:            false,
			wantWebhookInitErr: false,
			setupConfig: funcs{
				wConfig("admission_controller.auto_instrumentation.inject_auto_detected_libraries", true),
				enableAPMInstrumentation,
				withLibVersions(map[string]string{"ruby": "v1.2.3"}),
			},
		},
		{
			name: "Single Step Instrumentation: enable ASM",
			pod: common.FakePodSpec{
				ParentKind: "replicaset",
				ParentName: "test-app-123",
			}.Create(),
			expectedEnvs: append(append(injectAllEnvs(), expBasicConfig()...),
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_single_step",
				},
				corev1.EnvVar{
					Name:  "DD_SERVICE",
					Value: "test-app",
				},
				corev1.EnvVar{
					Name:  "DD_SERVICE_K8S_ENV_SOURCE",
					Value: "owner=test-app",
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
				corev1.EnvVar{
					Name:  "DD_APPSEC_ENABLED",
					Value: "true",
				},
			),
			expectedInjectedLibraries: defaultLibraries,
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig: funcs{
				enableAPMInstrumentation,
				wConfig("admission_controller.auto_instrumentation.asm.enabled", true),
			},
		},
		{
			name: "Single Step Instrumentation: enable iast",
			pod: common.FakePodSpec{
				ParentKind: "replicaset",
				ParentName: "test-app-123",
			}.Create(),
			expectedEnvs: append(append(injectAllEnvs(), expBasicConfig()...),
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_single_step",
				},
				corev1.EnvVar{
					Name:  "DD_SERVICE",
					Value: "test-app",
				},
				corev1.EnvVar{
					Name:  "DD_SERVICE_K8S_ENV_SOURCE",
					Value: "owner=test-app",
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
				corev1.EnvVar{
					Name:  "DD_IAST_ENABLED",
					Value: "true",
				},
			),
			expectedInjectedLibraries: defaultLibraries,
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig: funcs{
				enableAPMInstrumentation,
				wConfig("admission_controller.auto_instrumentation.iast.enabled", true),
			},
		},
		{
			name: "Single Step Instrumentation: disable sca",
			pod: common.FakePodSpec{
				ParentKind: "replicaset",
				ParentName: "test-app-123",
			}.Create(),
			expectedEnvs: append(append(injectAllEnvs(), expBasicConfig()...),
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_single_step",
				},
				corev1.EnvVar{
					Name:  "DD_SERVICE",
					Value: "test-app",
				},
				corev1.EnvVar{
					Name:  "DD_SERVICE_K8S_ENV_SOURCE",
					Value: "owner=test-app",
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
				corev1.EnvVar{
					Name:  "DD_APPSEC_SCA_ENABLED",
					Value: "false",
				},
			),
			expectedInjectedLibraries: defaultLibraries,
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig: funcs{
				enableAPMInstrumentation,
				wConfig("admission_controller.auto_instrumentation.asm_sca.enabled", false),
			},
		},
		{
			name: "Single Step Instrumentation: enable profiling",
			pod: common.FakePodSpec{
				ParentKind: "replicaset",
				ParentName: "test-app-123",
			}.Create(),
			expectedEnvs: append(append(injectAllEnvs(), expBasicConfig()...),
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_single_step",
				},
				corev1.EnvVar{
					Name:  "DD_SERVICE",
					Value: "test-app",
				},
				corev1.EnvVar{
					Name:  "DD_SERVICE_K8S_ENV_SOURCE",
					Value: "owner=test-app",
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				corev1.EnvVar{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
				corev1.EnvVar{
					Name:  "DD_PROFILING_ENABLED",
					Value: "auto",
				},
			),
			expectedInjectedLibraries: defaultLibraries,
			wantErr:                   false,
			setupConfig: funcs{
				enableAPMInstrumentation,
				wConfig("admission_controller.auto_instrumentation.profiling.enabled", "auto"),
			},
		},
		{
			name: "inject all with full security context",
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.version":   "latest",
					"admission.datadoghq.com/all-lib.config.v1": `{"version":1,"runtime_metrics_enabled":true,"tracing_rate_limit":50,"tracing_sampling_rate":0.3}`,
				},
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "true",
				},
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_lib_injection",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_RATE_LIMIT",
					Value: "50",
				},
				{
					Name:  "DD_TRACE_SAMPLE_RATE",
					Value: "0.30",
				},
				{
					Name:  "PYTHONPATH",
					Value: "/datadog-lib/",
				},
				{
					Name:  "RUBYOPT",
					Value: " -r/datadog-lib/auto_inject",
				},
				{
					Name:  "NODE_OPTIONS",
					Value: " --require=/datadog-lib/node_modules/dd-trace/init",
				},
				{
					Name:  "JAVA_TOOL_OPTIONS",
					Value: " -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log",
				},
				{
					Name:  "DD_DOTNET_TRACER_HOME",
					Value: "/datadog-lib",
				},
				{
					Name:  "CORECLR_ENABLE_PROFILING",
					Value: "1",
				},
				{
					Name:  "CORECLR_PROFILER",
					Value: "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}",
				},
				{
					Name:  "CORECLR_PROFILER_PATH",
					Value: "/datadog-lib/Datadog.Trace.ClrProfiler.Native.so",
				},
				{
					Name:  "DD_TRACE_LOG_DIRECTORY",
					Value: "/datadog-lib/logs",
				},
			},
			expectedInjectedLibraries: defaultLibraries,
			expectedSecurityContext: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Add:  []corev1.Capability{"NET_ADMIN", "SYS_TIME"},
					Drop: []corev1.Capability{"ALL"},
				},
				Privileged: pointer.Ptr(false),
				SELinuxOptions: &corev1.SELinuxOptions{
					User:  "test",
					Role:  "root",
					Type:  "none",
					Level: "s0:c123,c456",
				},
				WindowsOptions: &corev1.WindowsSecurityContextOptions{
					GMSACredentialSpecName: pointer.Ptr("Developer"),
					GMSACredentialSpec:     pointer.Ptr("http://localhost:8081"),
					RunAsUserName:          pointer.Ptr("Developer"),
					HostProcess:            pointer.Ptr(false),
				},
				RunAsUser:                pointer.Ptr(int64(1001)),
				RunAsGroup:               pointer.Ptr(int64(5)),
				RunAsNonRoot:             pointer.Ptr(true),
				ReadOnlyRootFilesystem:   pointer.Ptr(true),
				AllowPrivilegeEscalation: pointer.Ptr(false),
				ProcMount:                pointer.Ptr(corev1.DefaultProcMount),
				SeccompProfile: &corev1.SeccompProfile{
					Type:             "LocalHost",
					LocalhostProfile: pointer.Ptr("my-profiles/profile-allow.json"),
				},
			},
			wantErr:            false,
			wantWebhookInitErr: false,
			setupConfig: funcs{
				withInitSecurityConfig(`{"capabilities":{"add":["NET_ADMIN","SYS_TIME"],"drop":["ALL"]},"privileged":false,"seLinuxOptions":{"user":"test","role":"root","level":"s0:c123,c456","type":"none"},"windowsOptions":{"gmsaCredentialSpecName":"Developer","gmsaCredentialSpec":"http://localhost:8081","runAsUserName":"Developer","hostProcess":false},"runAsUser":1001,"runAsGroup":5,"runAsNonRoot":true,"readOnlyRootFilesystem":true,"allowPrivilegeEscalation":false,"procMount":"Default","seccompProfile":{"type":"LocalHost","localHostProfile":"my-profiles/profile-allow.json"}}`),
			},
		},
		{
			name: `inject all with pod security standard "restricted" compliant security context`,
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.version":   "latest",
					"admission.datadoghq.com/all-lib.config.v1": `{"version":1,"runtime_metrics_enabled":true,"tracing_rate_limit":50,"tracing_sampling_rate":0.3}`,
				},
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "true",
				},
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_lib_injection",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_RATE_LIMIT",
					Value: "50",
				},
				{
					Name:  "DD_TRACE_SAMPLE_RATE",
					Value: "0.30",
				},
				{
					Name:  "PYTHONPATH",
					Value: "/datadog-lib/",
				},
				{
					Name:  "RUBYOPT",
					Value: " -r/datadog-lib/auto_inject",
				},
				{
					Name:  "NODE_OPTIONS",
					Value: " --require=/datadog-lib/node_modules/dd-trace/init",
				},
				{
					Name:  "JAVA_TOOL_OPTIONS",
					Value: " -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log",
				},
				{
					Name:  "DD_DOTNET_TRACER_HOME",
					Value: "/datadog-lib",
				},
				{
					Name:  "CORECLR_ENABLE_PROFILING",
					Value: "1",
				},
				{
					Name:  "CORECLR_PROFILER",
					Value: "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}",
				},
				{
					Name:  "CORECLR_PROFILER_PATH",
					Value: "/datadog-lib/Datadog.Trace.ClrProfiler.Native.so",
				},
				{
					Name:  "DD_TRACE_LOG_DIRECTORY",
					Value: "/datadog-lib/logs",
				},
			},
			expectedInjectedLibraries: defaultLibraries,
			expectedSecurityContext: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
				RunAsNonRoot:             pointer.Ptr(true),
				ReadOnlyRootFilesystem:   pointer.Ptr(true),
				AllowPrivilegeEscalation: pointer.Ptr(false),
				SeccompProfile: &corev1.SeccompProfile{
					Type: "RuntimeDefault",
				},
			},
			wantErr:            false,
			wantWebhookInitErr: false,
			setupConfig: funcs{
				withInitSecurityConfig(`{"capabilities":{"drop":["ALL"]},"runAsNonRoot":true,"readOnlyRootFilesystem":true,"allowPrivilegeEscalation":false,"seccompProfile":{"type":"RuntimeDefault"}}`),
			},
		},
		{
			name: `inject all with ignoring unknown JSON properties in security context`,
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.version":   "latest",
					"admission.datadoghq.com/all-lib.config.v1": `{"version":1,"runtime_metrics_enabled":true,"tracing_rate_limit":50,"tracing_sampling_rate":0.3}`,
				},
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "true",
				},
			}.Create(),
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
					Value: "k8s_lib_injection",
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_RATE_LIMIT",
					Value: "50",
				},
				{
					Name:  "DD_TRACE_SAMPLE_RATE",
					Value: "0.30",
				},
				{
					Name:  "PYTHONPATH",
					Value: "/datadog-lib/",
				},
				{
					Name:  "RUBYOPT",
					Value: " -r/datadog-lib/auto_inject",
				},
				{
					Name:  "NODE_OPTIONS",
					Value: " --require=/datadog-lib/node_modules/dd-trace/init",
				},
				{
					Name:  "JAVA_TOOL_OPTIONS",
					Value: " -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log",
				},
				{
					Name:  "DD_DOTNET_TRACER_HOME",
					Value: "/datadog-lib",
				},
				{
					Name:  "CORECLR_ENABLE_PROFILING",
					Value: "1",
				},
				{
					Name:  "CORECLR_PROFILER",
					Value: "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}",
				},
				{
					Name:  "CORECLR_PROFILER_PATH",
					Value: "/datadog-lib/Datadog.Trace.ClrProfiler.Native.so",
				},
				{
					Name:  "DD_TRACE_LOG_DIRECTORY",
					Value: "/datadog-lib/logs",
				},
			},
			expectedInjectedLibraries: defaultLibraries,
			expectedSecurityContext:   &corev1.SecurityContext{},
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig: funcs{
				withInitSecurityConfig(`{"unknownProperty":true}`),
			},
		},
		{
			name: `invalid security context`,
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.version":   "latest",
					"admission.datadoghq.com/all-lib.config.v1": `{"version":1,"runtime_metrics_enabled":true,"tracing_rate_limit":50,"tracing_sampling_rate":0.3}`,
				},
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "true",
				},
			}.Create(),
			wantErr:            false,
			wantWebhookInitErr: true,
			setupConfig: funcs{
				withInitSecurityConfig(`{"privileged":"not a boolean"}`),
			},
		},
		{
			name: `invalid security context - bad json`,
			pod: common.FakePodSpec{
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.version":   "latest",
					"admission.datadoghq.com/all-lib.config.v1": `{"version":1,"runtime_metrics_enabled":true,"tracing_rate_limit":50,"tracing_sampling_rate":0.3}`,
				},
				Labels: map[string]string{
					"admission.datadoghq.com/enabled": "true",
				},
			}.Create(),
			wantErr:            false,
			wantWebhookInitErr: true,
			setupConfig: funcs{
				withInitSecurityConfig(`{"privileged":"not a boolean"`),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DD_INSTRUMENTATION_INSTALL_ID", uuid)
			t.Setenv("DD_INSTRUMENTATION_INSTALL_TIME", installTime)

			wmeta := common.FakeStoreWithDeployment(t, tt.langDetectionDeployments)

			mockConfig = configmock.New(t)
			mockConfig.SetWithoutSource("apm_config.instrumentation.version", "v1")
			if tt.setupConfig != nil {
				for _, f := range tt.setupConfig {
					f()
				}
			}

			// N.B. Force v1 for these tests!
			webhook, errInitAPMInstrumentation := maybeWebhook(wmeta, mockConfig)
			if tt.wantWebhookInitErr {
				require.Error(t, errInitAPMInstrumentation)
				return
			}

			require.NoError(t, errInitAPMInstrumentation)

			_, err := webhook.inject(tt.pod, tt.pod.Namespace, fake.NewSimpleDynamicClient(scheme.Scheme))
			require.False(t, (err != nil) != tt.wantErr)

			container := tt.pod.Spec.Containers[0]
			for _, contEnv := range container.Env {
				found := false
				for _, expectEnv := range tt.expectedEnvs {
					if expectEnv.Name == contEnv.Name {
						found = true
						break
					}
				}
				if !found {
					require.Failf(t, "Unexpected env var injected in container", "env=%+v", contEnv)
				}
			}

			for _, expectEnv := range tt.expectedEnvs {
				found := false
				for _, contEnv := range container.Env {
					if expectEnv.Name == contEnv.Name {
						found = true
						break
					}
				}
				if !found {
					require.Failf(t, "Expected env var injected in container not found", expectEnv.Name)
				}
			}

			envCount := 0
			for _, contEnv := range container.Env {
				for _, expectEnv := range tt.expectedEnvs {
					if expectEnv.Name == contEnv.Name {
						require.Equalf(t, expectEnv.Value, contEnv.Value, "for envvar %s", expectEnv.Name)
						envCount++
						break
					}
				}
			}
			require.Equal(t, len(tt.expectedEnvs), envCount)

			initContainers := tt.pod.Spec.InitContainers
			require.Equal(t, len(tt.expectedInjectedLibraries), len(initContainers))
			for _, c := range initContainers {
				language := getLanguageFromInitContainerName(c.Name)
				require.Contains(t,
					tt.expectedInjectedLibraries, language,
					"unexpected injected language %s", language)
				require.Equal(t,
					tt.expectedInjectedLibraries[language], strings.Split(c.Image, ":")[1],
					"unexpected language version %s", language)
				require.Equal(t,
					tt.expectedSecurityContext, c.SecurityContext,
					"unexpected security context for language %s", language)
			}
		})
	}
}

func getLanguageFromInitContainerName(initContainerName string) string {
	trimmedSuffix := strings.TrimSuffix(initContainerName, "-init")
	return strings.TrimPrefix(trimmedSuffix, "datadog-lib-")
}

func TestShouldInject(t *testing.T) {
	var mockConfig model.Config
	tests := []struct {
		name        string
		pod         *corev1.Pod
		setupConfig func()
		want        bool
	}{
		{
			name:        "instrumentation on, no label",
			pod:         common.FakePodWithNamespaceAndLabel("ns", "", ""),
			setupConfig: func() { mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true) },
			want:        true,
		},
		{
			name:        "instrumentation on, label disabled",
			pod:         common.FakePodWithNamespaceAndLabel("ns", "admission.datadoghq.com/enabled", "false"),
			setupConfig: func() { mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true) },
			want:        false,
		},
		{
			name: "instrumentation on with disabled namespace, no label ns",
			pod:  common.FakePodWithNamespaceAndLabel("ns", "", ""),
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true)
				mockConfig.SetWithoutSource("apm_config.instrumentation.disabled_namespaces", []string{"ns"})
			},
			want: false,
		},
		{
			name: "instrumentation on with disabled namespace, no label ns2",
			pod:  common.FakePodWithNamespaceAndLabel("ns2", "", ""),
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true)
				mockConfig.SetWithoutSource("apm_config.instrumentation.disabled_namespaces", []string{"ns"})
			},
			want: true,
		},
		{
			name: "instrumentation on with disabled namespace, disabled label",
			pod:  common.FakePodWithNamespaceAndLabel("ns", "admission.datadoghq.com/enabled", "false"),
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true)
				mockConfig.SetWithoutSource("apm_config.instrumentation.disabled_namespaces", []string{"ns"})
			},
			want: false,
		},
		{
			name: "instrumentation on with disabled namespace, label enabled",
			pod:  common.FakePodWithNamespaceAndLabel("ns", "admission.datadoghq.com/enabled", "true"),
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true)
				mockConfig.SetWithoutSource("apm_config.instrumentation.disabled_namespaces", []string{"ns"})
			},
			want: true,
		},
		{
			name:        "instrumentation off, label enabled",
			pod:         common.FakePodWithNamespaceAndLabel("ns", "admission.datadoghq.com/enabled", "true"),
			setupConfig: func() { mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", false) },
			want:        true,
		},
		{
			name:        "instrumentation off, no label",
			pod:         common.FakePodWithNamespaceAndLabel("ns", "", ""),
			setupConfig: func() { mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", false) },
			want:        false,
		},
		{
			name: "instrumentation off with enabled namespace, label enabled",
			pod:  common.FakePodWithNamespaceAndLabel("ns", "admission.datadoghq.com/enabled", "true"),
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true)
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled_namespaces", []string{"ns"})
			},
			want: true,
		},
		{
			name: "instrumentation off with enabled namespace, label disabled",
			pod:  common.FakePodWithNamespaceAndLabel("ns", "admission.datadoghq.com/enabled", "false"),
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", false)
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled_namespaces", []string{"ns"})
			},
			want: false,
		},
		{
			name: "instrumentation off with enabled namespace, no label",
			pod:  common.FakePodWithNamespaceAndLabel("ns", "", ""),
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", false)
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled_namespaces", []string{"ns"})
			},
			want: false,
		},
		{
			name: "instrumentation on with enabled namespace, no label",
			pod:  common.FakePodWithNamespaceAndLabel("ns", "", ""),
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true)
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled_namespaces", []string{"ns"})
			},
			want: true,
		},
		{
			name: "instrumentation on with enabled other namespace, no label",
			pod:  common.FakePodWithNamespaceAndLabel("ns2", "", ""),
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true)
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled_namespaces", []string{"ns"})
			},
			want: false,
		},
		{
			name: "instrumentation on in kube-system namespace, no label",
			pod:  common.FakePodWithNamespaceAndLabel("kube-system", "", ""),
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true)
			},
			want: false,
		},
		{
			name: "instrumentation on in default (datadog) namespace, no label",
			pod:  common.FakePodWithNamespaceAndLabel("default", "", ""),
			setupConfig: func() {
				mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", true)
				mockConfig.SetWithoutSource("kube_resources_namespace", "default")
			},
			want: false,
		},
		{
			name:        "Mutate unlabelled, no label",
			pod:         common.FakePodWithLabel("", ""),
			setupConfig: func() { mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", true) },
			want:        true,
		},
		{
			name:        "Mutate unlabelled, label enabled",
			pod:         common.FakePodWithLabel("admission.datadoghq.com/enabled", "true"),
			setupConfig: func() { mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", true) },
			want:        true,
		},
		{
			name:        "Mutate unlabelled, label disabled",
			pod:         common.FakePodWithLabel("admission.datadoghq.com/enabled", "false"),
			setupConfig: func() { mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", true) },
			want:        false,
		},
		{
			name:        "no Mutate unlabelled, no label",
			pod:         common.FakePodWithLabel("", ""),
			setupConfig: func() { mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", false) },
			want:        false,
		},
		{
			name:        "no Mutate unlabelled, label enabled",
			pod:         common.FakePodWithLabel("admission.datadoghq.com/enabled", "true"),
			setupConfig: func() { mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", false) },
			want:        true,
		},
		{
			name:        "no Mutate unlabelled, label disabled",
			pod:         common.FakePodWithLabel("admission.datadoghq.com/enabled", "false"),
			setupConfig: func() { mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", false) },
			want:        false,
		},
		{
			name:        "no Mutate unlabelled, label disabled",
			pod:         common.FakePodWithLabel("admission.datadoghq.com/enabled", "false"),
			setupConfig: func() { mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", false) },
			want:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wmeta := fxutil.Test[workloadmeta.Component](t,
				core.MockBundle(),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			)

			mockConfig = configmock.New(t)
			tt.setupConfig()

			config, err := NewConfig(mockConfig)
			require.NoError(t, err)
			mutator, err := NewNamespaceMutator(config, wmeta)
			require.NoError(t, err)
			require.Equal(t, tt.want, mutator.isPodEligible(tt.pod), "expected webhook.isPodEligible() to be %t", tt.want)
		})
	}
}

func maybeWebhook(wmeta workloadmeta.Component, ddConfig config.Component) (*Webhook, error) {
	config, err := NewConfig(ddConfig)
	if err != nil {
		return nil, err
	}

	mutator, err := NewNamespaceMutator(config, wmeta)
	if err != nil {
		return nil, err
	}
	webhook, err := NewWebhook(config, wmeta, mutator)
	if err != nil {
		return nil, err
	}

	return webhook, nil
}

func languageSetOf(languages ...string) languagemodels.LanguageSet {
	set := languagemodels.LanguageSet{}
	for _, l := range languages {
		_ = set.Add(languagemodels.LanguageName(l))
	}
	return set
}

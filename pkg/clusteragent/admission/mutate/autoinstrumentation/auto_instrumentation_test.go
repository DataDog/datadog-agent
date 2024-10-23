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
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const commonRegistry = "gcr.io/datadoghq"

func TestInjectAutoInstruConfigV2(t *testing.T) {
	tests := []struct {
		name                    string
		pod                     *corev1.Pod
		libInfo                 extractedPodLibInfo
		expectedInjectorImage   string
		expectedLangsDetected   string
		expectedInstallType     string
		expectedSecurityContext *corev1.SecurityContext
		wantErr                 bool
		config                  func(c model.Config)
	}{
		{
			name: "no libs, no injection",
			pod:  common.FakePod("java-pod"),
		},
		{
			name:                    "nominal case: java",
			pod:                     common.FakePod("java-pod"),
			expectedInjectorImage:   commonRegistry + "/apm-inject:0",
			expectedSecurityContext: &corev1.SecurityContext{},
			libInfo: extractedPodLibInfo{
				libs: []libInfo{
					java.libInfo("", "gcr.io/datadoghq/dd-lib-java-init:v1"),
				},
			},
		},
		{
			name:                    "nominal case: java & python",
			pod:                     common.FakePod("java-pod"),
			expectedInjectorImage:   commonRegistry + "/apm-inject:0",
			expectedSecurityContext: &corev1.SecurityContext{},
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
			expectedInjectorImage:   commonRegistry + "/apm-inject:v0",
			expectedSecurityContext: &corev1.SecurityContext{},
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
			expectedInjectorImage:   "docker.io/library/apm-inject-package:v27",
			expectedSecurityContext: &corev1.SecurityContext{},
			libInfo: extractedPodLibInfo{
				libs: []libInfo{
					java.libInfo("", "gcr.io/datadoghq/dd-lib-java-init:v1"),
				},
			},
		},
		{
			name:                    "config injector-image-override",
			pod:                     common.FakePod("java-pod"),
			expectedInjectorImage:   "gcr.io/datadoghq/apm-inject:0.16-1",
			expectedSecurityContext: &corev1.SecurityContext{},
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
			name:                    "config language detected env vars",
			pod:                     common.FakePod("java-pod"),
			expectedInjectorImage:   "gcr.io/datadoghq/apm-inject:0.16-1",
			expectedLangsDetected:   "python",
			expectedInstallType:     "k8s_lib_injection",
			expectedSecurityContext: &corev1.SecurityContext{},
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
			name:                    "language detected for a different container",
			pod:                     common.FakePod("java-pod"),
			expectedInjectorImage:   "gcr.io/datadoghq/apm-inject:0",
			expectedSecurityContext: &corev1.SecurityContext{},
			expectedLangsDetected:   "",
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
			name:                    "language detected but no languages found",
			pod:                     common.FakePod("java-pod"),
			expectedInjectorImage:   "gcr.io/datadoghq/apm-inject:0",
			expectedSecurityContext: &corev1.SecurityContext{},
			expectedLangsDetected:   "",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wmeta := common.FakeStoreWithDeployment(t, nil)

			c := configmock.New(t)

			c.SetWithoutSource("apm_config.instrumentation.version", "v2")
			if tt.config != nil {
				tt.config(c)
			}

			webhook := mustWebhook(t, wmeta, c)
			require.Equal(t, instrumentationV2, webhook.config.version)
			require.True(t, webhook.config.version.usesInjector())

			webhook.config.initSecurityContext = tt.expectedSecurityContext

			if tt.libInfo.source == libInfoSourceNone {
				tt.libInfo.source = libInfoSourceSingleStepInstrumentation
			}

			if tt.expectedInstallType == "" {
				tt.expectedInstallType = "k8s_single_step"
			}

			err := webhook.injectAutoInstruConfig(tt.pod, tt.libInfo)

			if tt.wantErr {
				require.Error(t, err, "expected injectAutoInstruConfig to error")
			} else {
				require.NoError(t, err, "expected injectAutoInstruConfig to succeed")
			}

			if err != nil {
				return
			}

			envsByName := map[string]corev1.EnvVar{}
			for _, env := range tt.pod.Spec.Containers[0].Env {
				envsByName[env.Name] = env
			}

			requireEnv := func(t *testing.T, key string, ok bool, value string) {
				t.Helper()
				val, exists := envsByName[key]
				require.Equal(t, ok, exists, "expected env %v exists to = %v", key, ok)
				require.Equal(t, value, val.Value, "expected env %v = %v", key, val)
			}

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
				ReadOnly:  true,
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

			requireEnv(t, "LD_PRELOAD", true, "/opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so")
			requireEnv(t, "DD_INJECT_SENDER_TYPE", true, "k8s")

			requireEnv(t, "DD_INSTRUMENTATION_INSTALL_TYPE", true, tt.expectedInstallType)

			if tt.libInfo.languageDetection == nil {
				requireEnv(t, "DD_INSTRUMENTATION_LANGUAGES_DETECTED", false, "")
				requireEnv(t, "DD_INSTRUMENTATION_LANGUAGE_DETECTION_INJECTION_ENABLED", false, "")
			} else {
				requireEnv(t, "DD_INSTRUMENTATION_LANGUAGES_DETECTED", true, tt.expectedLangsDetected)
				requireEnv(t, "DD_INSTRUMENTATION_LANGUAGE_DETECTION_INJECTION_ENABLED", true, strconv.FormatBool(tt.libInfo.languageDetection.injectionEnabled))
			}
		})
	}
}

func TestInjectAutoInstruConfig(t *testing.T) {
	tests := []struct {
		name           string
		pod            *corev1.Pod
		libsToInject   []libInfo
		expectedEnvKey string
		expectedEnvVal string
		wantErr        bool
	}{
		{
			name: "nominal case: java",
			pod:  common.FakePod("java-pod"),
			libsToInject: []libInfo{
				{
					lang:  "java",
					image: "gcr.io/datadoghq/dd-lib-java-init:v1",
				},
			},
			expectedEnvKey: "JAVA_TOOL_OPTIONS",
			expectedEnvVal: " -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log",
			wantErr:        false,
		},
		{
			name: "JAVA_TOOL_OPTIONS not empty",
			pod:  common.FakePodWithEnvValue("java-pod", "JAVA_TOOL_OPTIONS", "predefined"),
			libsToInject: []libInfo{
				{
					lang:  "java",
					image: "gcr.io/datadoghq/dd-lib-java-init:v1",
				},
			},
			expectedEnvKey: "JAVA_TOOL_OPTIONS",
			expectedEnvVal: "predefined -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log",
			wantErr:        false,
		},
		{
			name: "JAVA_TOOL_OPTIONS set via ValueFrom",
			pod:  common.FakePodWithEnvFieldRefValue("java-pod", "JAVA_TOOL_OPTIONS", "path"),
			libsToInject: []libInfo{
				{
					lang:  "java",
					image: "gcr.io/datadoghq/dd-lib-java-init:v1",
				},
			},
			wantErr: true,
		},
		{
			name: "nominal case: js",
			pod:  common.FakePod("js-pod"),
			libsToInject: []libInfo{
				{
					lang:  "js",
					image: "gcr.io/datadoghq/dd-lib-js-init:v1",
				},
			},
			expectedEnvKey: "NODE_OPTIONS",
			expectedEnvVal: " --require=/datadog-lib/node_modules/dd-trace/init",
			wantErr:        false,
		},
		{
			name: "NODE_OPTIONS not empty",
			pod:  common.FakePodWithEnvValue("js-pod", "NODE_OPTIONS", "predefined"),
			libsToInject: []libInfo{
				{
					lang:  "js",
					image: "gcr.io/datadoghq/dd-lib-js-init:v1",
				},
			},
			expectedEnvKey: "NODE_OPTIONS",
			expectedEnvVal: "predefined --require=/datadog-lib/node_modules/dd-trace/init",
			wantErr:        false,
		},
		{
			name: "NODE_OPTIONS set via ValueFrom",
			pod:  common.FakePodWithEnvFieldRefValue("js-pod", "NODE_OPTIONS", "path"),
			libsToInject: []libInfo{
				{
					lang:  "js",
					image: "gcr.io/datadoghq/dd-lib-js-init:v1",
				},
			},
			wantErr: true,
		},
		{
			name: "nominal case: python",
			pod:  common.FakePod("python-pod"),
			libsToInject: []libInfo{
				{
					lang:  "python",
					image: "gcr.io/datadoghq/dd-lib-python-init:v1",
				},
			},
			expectedEnvKey: "PYTHONPATH",
			expectedEnvVal: "/datadog-lib/",
			wantErr:        false,
		},
		{
			name: "PYTHONPATH not empty",
			pod:  common.FakePodWithEnvValue("python-pod", "PYTHONPATH", "predefined"),
			libsToInject: []libInfo{
				{
					lang:  "python",
					image: "gcr.io/datadoghq/dd-lib-python-init:v1",
				},
			},
			expectedEnvKey: "PYTHONPATH",
			expectedEnvVal: "/datadog-lib/:predefined",
			wantErr:        false,
		},
		{
			name: "PYTHONPATH set via ValueFrom",
			pod:  common.FakePodWithEnvFieldRefValue("python-pod", "PYTHONPATH", "path"),
			libsToInject: []libInfo{
				{
					lang:  "python",
					image: "gcr.io/datadoghq/dd-lib-python-init:v1",
				},
			},
			wantErr: true,
		},
		{
			name: "Unknown language",
			pod:  common.FakePod("unknown-pod"),
			libsToInject: []libInfo{
				{
					lang:  "unknown",
					image: "gcr.io/datadoghq/dd-lib-unknown-init:v1",
				},
			},
			wantErr: true,
		},
		{
			name: "nominal case: dotnet",
			pod:  common.FakePod("dotnet-pod"),
			libsToInject: []libInfo{
				{
					lang:  "dotnet",
					image: "gcr.io/datadoghq/dd-lib-dotnet-init:v1",
				},
			},
			expectedEnvKey: "CORECLR_PROFILER",
			expectedEnvVal: "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}",
			wantErr:        false,
		},
		{
			name: "CORECLR_ENABLE_PROFILING not empty",
			pod:  common.FakePodWithEnvValue("dotnet-pod", "CORECLR_PROFILER", "predefined"),
			libsToInject: []libInfo{
				{
					lang:  "dotnet",
					image: "gcr.io/datadoghq/dd-lib-dotnet-init:v1",
				},
			},
			expectedEnvKey: "CORECLR_PROFILER",
			expectedEnvVal: "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}",
			wantErr:        false,
		},
		{
			name: "CORECLR_ENABLE_PROFILING set via ValueFrom",
			pod:  common.FakePodWithEnvFieldRefValue("dotnet-pod", "CORECLR_PROFILER", "path"),
			libsToInject: []libInfo{
				{
					lang:  dotnet,
					image: "gcr.io/datadoghq/dd-lib-dotnet-init:v1",
				},
			},
			wantErr: true,
		},
		{
			name: "nominal case: ruby",
			pod:  common.FakePod("ruby-pod"),
			libsToInject: []libInfo{
				{
					lang:  ruby,
					image: "gcr.io/datadoghq/dd-lib-ruby-init:v1",
				},
			},
			expectedEnvKey: "RUBYOPT",
			expectedEnvVal: " -r/datadog-lib/auto_inject",
			wantErr:        false,
		},
		{
			name: "RUBYOPT not empty",
			pod:  common.FakePodWithEnvValue("ruby-pod", "RUBYOPT", "predefined"),
			libsToInject: []libInfo{
				{
					lang:  "ruby",
					image: "gcr.io/datadoghq/dd-lib-ruby-init:v1",
				},
			},
			expectedEnvKey: "RUBYOPT",
			expectedEnvVal: "predefined -r/datadog-lib/auto_inject",
			wantErr:        false,
		},
		{
			name: "RUBYOPT set via ValueFrom",
			pod:  common.FakePodWithEnvFieldRefValue("ruby-pod", "RUBYOPT", "path"),
			libsToInject: []libInfo{
				{
					lang:  "ruby",
					image: "gcr.io/datadoghq/dd-lib-ruby-init:v1",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wmeta := fxutil.Test[workloadmeta.Component](t,
				core.MockBundle(),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			)

			c := configmock.New(t)
			c.SetWithoutSource("apm_config.instrumentation.version", "v1")

			webhook := mustWebhook(t, wmeta, c)
			err := webhook.injectAutoInstruConfig(tt.pod, extractedPodLibInfo{
				libs:   tt.libsToInject,
				source: libInfoSourceLibInjection,
			})
			if tt.wantErr {
				require.Error(t, err, "expected injectAutoInstruConfig to error")
			} else {
				require.NoError(t, err, "expected injectAutoInstruConfig to succeed")
			}

			if err != nil {
				return
			}

			assertLibReq(t, tt.pod, tt.libsToInject[0].lang, tt.libsToInject[0].image, tt.expectedEnvKey, tt.expectedEnvVal)
		})
	}
}

func assertLibReq(t *testing.T, pod *corev1.Pod, lang language, image, envKey, envVal string) {
	// Empty dir volume
	volumeFound := false
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == "datadog-auto-instrumentation" {
			require.NotNil(t, volume.VolumeSource.EmptyDir)
			volumeFound = true
			break
		}
	}
	require.True(t, volumeFound)

	// Init container
	initContainerFound := false
	for _, container := range pod.Spec.InitContainers {
		if container.Name == fmt.Sprintf("datadog-lib-%s-init", lang) {
			require.Equal(t, image, container.Image)
			require.Equal(t, []string{"sh", "copy-lib.sh", "/datadog-lib"}, container.Command)
			require.Equal(t, "datadog-auto-instrumentation", container.VolumeMounts[0].Name)
			require.Equal(t, "/datadog-lib", container.VolumeMounts[0].MountPath)
			initContainerFound = true
			break
		}
	}
	require.True(t, initContainerFound)

	// App container
	container := pod.Spec.Containers[0]
	require.Equal(t, "datadog-auto-instrumentation", container.VolumeMounts[0].Name)
	require.Equal(t, "/datadog-lib", container.VolumeMounts[0].MountPath)
	envFound := false
	for _, env := range container.Env {
		if env.Name == envKey {
			require.Equal(t, envVal, env.Value)
			envFound = true
			break
		}
	}
	require.True(t, envFound, "expected to find env %s with value %s", envKey, envVal)
}

func TestExtractLibInfo(t *testing.T) {
	// TODO: Add new entry when a new language is supported
	allLatestLibs := []libInfo{
		{
			lang:  "java",
			image: "registry/dd-lib-java-init:v1",
		},
		{
			lang:  "js",
			image: "registry/dd-lib-js-init:v5",
		},
		{
			lang:  "python",
			image: "registry/dd-lib-python-init:v2",
		},
		{
			lang:  "dotnet",
			image: "registry/dd-lib-dotnet-init:v3",
		},
		{
			lang:  "ruby",
			image: "registry/dd-lib-ruby-init:v2",
		},
	}

	var mockConfig model.Config
	tests := []struct {
		name                 string
		pod                  *corev1.Pod
		containerRegistry    string
		expectedLibsToInject []libInfo
		expectedPodEligible  *bool
		setupConfig          func()
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
			expectedLibsToInject: allLatestLibs,
		},
		{
			name:                 "all with mutate_unlabelled off",
			pod:                  common.FakePodWithAnnotation("admission.datadoghq.com/all-lib.version", "latest"),
			containerRegistry:    "registry",
			expectedPodEligible:  pointer.Ptr(false),
			expectedLibsToInject: allLatestLibs,
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
			expectedLibsToInject: allLatestLibs,
			setupConfig: func() {
				mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", false)
			},
		},
		{
			name:                 "all with mutate_unlabelled off",
			pod:                  common.FakePodWithAnnotation("admission.datadoghq.com/all-lib.version", "latest"),
			containerRegistry:    "registry",
			expectedPodEligible:  pointer.Ptr(false),
			expectedLibsToInject: allLatestLibs,
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
			expectedLibsToInject: allLatestLibs,
			setupConfig: func() {
				mockConfig.SetWithoutSource("admission_controller.mutate_unlabelled", false)
			},
		},
		{
			name:                 "all with mutate_unlabelled off",
			pod:                  common.FakePodWithAnnotation("admission.datadoghq.com/all-lib.version", "latest"),
			containerRegistry:    "registry",
			expectedPodEligible:  pointer.Ptr(false),
			expectedLibsToInject: allLatestLibs,
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
			expectedLibsToInject: allLatestLibs,
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
			expectedLibsToInject: allLatestLibs,
			setupConfig:          func() { mockConfig.SetWithoutSource("apm_config.instrumentation.enabled", false) },
		},
		{
			name:                 "single step instrumentation with no pinned versions",
			pod:                  common.FakePodWithNamespaceAndLabel("ns", "", ""),
			containerRegistry:    "registry",
			expectedLibsToInject: allLatestLibs,
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

			wmeta := fxutil.Test[workloadmeta.Component](t,
				core.MockBundle(),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			)
			mockConfig = configmock.New(t)
			for k, v := range overrides {
				mockConfig.SetWithoutSource(k, v)
			}

			if tt.setupConfig != nil {
				tt.setupConfig()
			}

			webhook := mustWebhook(t, wmeta, mockConfig)

			if tt.expectedPodEligible != nil {
				require.Equal(t, *tt.expectedPodEligible, webhook.isPodEligible(tt.pod))
			}

			extracted := webhook.extractLibInfo(tt.pod)
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
		name              string
		cpu               string
		mem               string
		pod               *corev1.Pod
		image             string
		lang              language
		wantSkipInjection bool
		wantErr           bool
		wantCPU           string
		wantMem           string
		secCtx            *corev1.SecurityContext
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
					corev1.ResourceCPU:    resource.MustParse("499"),
					corev1.ResourceMemory: resource.MustParse("101Mi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("499"),
					corev1.ResourceMemory: resource.MustParse("101Mi"),
				},
			}),
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "499",
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
							corev1.ResourceCPU:    resource.MustParse("499"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("499"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
					}}, {Name: "with_init_container_resources_init-2", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("501"),
							corev1.ResourceMemory: resource.MustParse("99Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("501"),
							corev1.ResourceMemory: resource.MustParse("99Mi"),
						},
					}}},
					Containers: []corev1.Container{{Name: "c1"}},
				},
			},
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "501",
			wantMem: "101Mi",
		},
		{
			name: "multiple_container_resources",
			pod: common.FakePodWithContainer("java-pod", corev1.Container{
				Name: "c1",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("499"),
						corev1.ResourceMemory: resource.MustParse("101Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("499"),
						corev1.ResourceMemory: resource.MustParse("101Mi"),
					},
				},
			}, corev1.Container{
				Name: "c2",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("499"),
						corev1.ResourceMemory: resource.MustParse("101Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("499"),
						corev1.ResourceMemory: resource.MustParse("101Mi"),
					},
				},
			}),
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "998",
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
							corev1.ResourceCPU:    resource.MustParse("501"),
							corev1.ResourceMemory: resource.MustParse("99Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("501"),
							corev1.ResourceMemory: resource.MustParse("99Mi"),
						},
					}}},
					Containers: []corev1.Container{{Name: "c1", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("499"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("499"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
					}}},
				},
			},
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "501",
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
							corev1.ResourceCPU:    resource.MustParse("501"),
							corev1.ResourceMemory: resource.MustParse("99Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("501"),
							corev1.ResourceMemory: resource.MustParse("99Mi"),
						},
					}}},
					Containers: []corev1.Container{{Name: "c1", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("499"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("499"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
					}}},
				},
			},
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "501",
			wantMem: "101Mi",
		},
		{
			name: "config_and_resources",
			pod: common.FakePodWithContainer("java-pod", corev1.Container{Name: "c1", Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("499"),
					corev1.ResourceMemory: resource.MustParse("101Mi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("499"),
					corev1.ResourceMemory: resource.MustParse("101Mi"),
				},
			}}),
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			cpu:     "100",
			mem:     "256Mi",
			wantErr: false,
			wantCPU: "100",
			wantMem: "256Mi",
		},
		{
			name: "low_memory_skip",
			pod: common.FakePodWithContainer("java-pod", corev1.Container{Name: "c1", Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("499"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("499"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
			}}),
			image:             "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:              java,
			wantErr:           false,
			wantSkipInjection: true,
		},
		{
			name: "low_cpu_skip",
			pod: common.FakePodWithContainer("java-pod", corev1.Container{Name: "c1", Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("25"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("25"),
				},
			}}),
			image:             "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:              java,
			wantErr:           false,
			wantSkipInjection: true,
		},
		{
			name: "config_override_low_limit_skip",
			pod: common.FakePodWithContainer("java-pod", corev1.Container{Name: "c1", Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("499"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("499"),
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
									corev1.ResourceCPU:    resource.MustParse("501"),
									corev1.ResourceMemory: resource.MustParse("101Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("501"),
									corev1.ResourceMemory: resource.MustParse("101Mi"),
								},
							},
						}, {
							Name:          "sidecar-container-1",
							RestartPolicy: pointer.Ptr(corev1.ContainerRestartPolicyAlways),
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500"),
									corev1.ResourceMemory: resource.MustParse("50Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500"),
									corev1.ResourceMemory: resource.MustParse("50Mi"),
								},
							},
						},
					},
					Containers: []corev1.Container{{Name: "c1", Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200"),
							corev1.ResourceMemory: resource.MustParse("101Mi"),
						},
					}}},
				},
			},
			image:   "gcr.io/datadoghq/dd-lib-java-init:v1",
			lang:    java,
			wantErr: false,
			wantCPU: "700",
			wantMem: "151Mi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wmeta := fxutil.Test[workloadmeta.Component](t,
				core.MockBundle(),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			)

			conf := configmock.New(t)
			if tt.cpu != "" {
				conf.SetWithoutSource("admission_controller.auto_instrumentation.init_resources.cpu", tt.cpu)
			}
			if tt.mem != "" {
				conf.SetWithoutSource("admission_controller.auto_instrumentation.init_resources.memory", tt.mem)
			}
			filter, _ := NewInjectionFilter(conf)
			wh, err := NewWebhook(wmeta, conf, filter)
			if (err != nil) != tt.wantErr {
				t.Errorf("injectLibInitContainer() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}

			// N.B. this is a bit hacky but consistent.
			wh.config.initSecurityContext = tt.secCtx

			c := tt.lang.libInfo("", tt.image).initContainers(wh.config.version)[0]
			requirements, skipInjection := initContainerResourceRequirements(tt.pod, wh.config.defaultResourceRequirements)
			require.Equal(t, tt.wantSkipInjection, skipInjection)
			if tt.wantSkipInjection {
				return
			}
			c.Mutators = wh.newContainerMutators(requirements)
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
			wantCPUQuantity := resource.MustParse(tt.wantCPU)
			t.Log(wantCPUQuantity, req)
			require.Zero(t, wantCPUQuantity.Cmp(req)) // Cmp returns 0 if equal
			require.Zero(t, wantCPUQuantity.Cmp(lim))

			req = tt.pod.Spec.InitContainers[initalInitContainerCount].Resources.Requests[corev1.ResourceMemory]
			lim = tt.pod.Spec.InitContainers[initalInitContainerCount].Resources.Limits[corev1.ResourceMemory]
			wantMemQuantity := resource.MustParse(tt.wantMem)
			require.Zero(t, wantMemQuantity.Cmp(req))
			require.Zero(t, wantMemQuantity.Cmp(lim))

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

func TestInjectAutoInstrumentation(t *testing.T) {
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

	defaultLibraries := map[string]string{
		"java":   "v1",
		"python": "v2",
		"ruby":   "v2",
		"dotnet": "v3",
		"js":     "v5",
	}

	defaultLibrariesFor := func(languages ...string) map[string]string {
		out := map[string]string{}
		for _, l := range languages {
			out[l] = defaultLibraries[l]
		}
		return out
	}

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
			name: "inject all with dotnet-profiler",
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
			},
			expectedInjectedLibraries: map[string]string{},
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
			},
			expectedInjectedLibraries: map[string]string{},
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedEnvs: append(append(injectAllEnvs(), expBasicConfig()...), corev1.EnvVar{
				Name:  "DD_SERVICE",
				Value: "test-deployment",
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedEnvs: append(append(injectAllEnvs(), expBasicConfig()...), corev1.EnvVar{
				Name:  "DD_SERVICE",
				Value: "test-statefulset-123",
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
			},
			expectedInjectedLibraries: map[string]string{},
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
					Value: installTime,
				},
				{
					Name:  "DD_INSTRUMENTATION_INSTALL_ID",
					Value: uuid,
				},
			},
			expectedInjectedLibraries: map[string]string{},
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedInjectedLibraries: map[string]string{"java": "v1.28.0", "python": "v2.5.1"},
			expectedSecurityContext:   &corev1.SecurityContext{},
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig: funcs{
				enableAPMInstrumentation,
				withLibVersions(map[string]string{"java": "v1.28.0", "python": "v2.5.1"}),
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
			expectedSecurityContext:   &corev1.SecurityContext{},
			wantErr:                   false,
			wantWebhookInitErr:        false,
			setupConfig: funcs{
				enableAPMInstrumentation,
				withLibVersions(map[string]string{"java": "v1.28.0", "python": "v2.5.1"}),
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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
			expectedSecurityContext:   &corev1.SecurityContext{},
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

			filter, _ := NewInjectionFilter(mockConfig)
			webhook, errInitAPMInstrumentation := NewWebhook(wmeta, mockConfig, filter)
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
					require.Failf(t, "Unexpected env var injected in container", contEnv.Name)
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

			webhook := mustWebhook(t, wmeta, mockConfig)
			require.Equal(t, tt.want, webhook.isPodEligible(tt.pod), "expected webhook.isPodEligible() to be %t", tt.want)
		})
	}
}

func mustWebhook(t *testing.T, wmeta workloadmeta.Component, ddConfig config.Component) *Webhook {
	filter, _ := NewInjectionFilter(ddConfig)
	webhook, err := NewWebhook(wmeta, ddConfig, filter)
	require.NoError(t, err)
	return webhook
}

func languageSetOf(languages ...string) util.LanguageSet {
	set := util.LanguageSet{}
	for _, l := range languages {
		_ = set.Add(util.Language(l))
	}
	return set
}

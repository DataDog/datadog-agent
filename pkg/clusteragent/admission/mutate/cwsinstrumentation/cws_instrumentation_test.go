// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cwsinstrumentation

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockK8sClientInterface is a mocked client that implements kubernetes.interface
type MockK8sClientInterface struct {
	kubernetes.Interface

	// annotations contains the annotations of the pod returned by the mocked client
	annotations map[string]string
	// shouldFail indicates if the mocked client should return an error
	shouldFail bool
	// containerName is the name of the first container for the fake generated pod
	containerName string
}

// CoreV1 retrieves the CoreV1Client
func (mkci MockK8sClientInterface) CoreV1() v1.CoreV1Interface {
	return &MockCoreV1Interface{annotations: mkci.annotations, containerName: mkci.containerName, shouldFail: mkci.shouldFail}
}

// MockCoreV1Interface is a mocked client that implements v1.CoreV1Interface
type MockCoreV1Interface struct {
	v1.CoreV1Interface

	// annotations contains the annotations of the pod returned by the mocked client
	annotations map[string]string
	// shouldFail indicates if the mocked client should return an error
	shouldFail bool
	// containerName is the name of the first container for the fake generated pod
	containerName string
}

// Pods returns an interface which has a method to return a PodInterface
func (mcvi MockCoreV1Interface) Pods(_ string) v1.PodInterface {
	return &MockV1PodsGetter{annotations: mcvi.annotations, containerName: mcvi.containerName, shouldFail: mcvi.shouldFail}
}

// Nodes returns an interface which has a method to return a NodeInterface
func (mcvi MockCoreV1Interface) Nodes() v1.NodeInterface {
	return &MockV1NodesGetter{}
}

// MockV1PodsGetter is a mocked client that implements v1.PodsGetter
type MockV1PodsGetter struct {
	v1.PodInterface

	// annotations contains the annotations of the pod returned by the mocked client
	annotations map[string]string
	// shouldFail indicates if the mocked client should return an error
	shouldFail bool
	// containerName is the name of the first container for the fake generated pod
	containerName string
}

// Get looks up a pod based on user input
func (mvpg *MockV1PodsGetter) Get(_ context.Context, _ string, _ metav1.GetOptions) (*corev1.Pod, error) {
	if mvpg.shouldFail {
		return nil, fmt.Errorf("mocked V1PodsGetter error")
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: mvpg.annotations,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: mvpg.containerName,
				},
			},
		},
	}, nil
}

// MockV1NodesGetter is a mocked client that implements v1.NodesGetter
type MockV1NodesGetter struct {
	v1.NodeInterface
}

// Get looks up a node based on user input
func (mvng *MockV1NodesGetter) Get(_ context.Context, _ string, _ metav1.GetOptions) (*corev1.Node, error) {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
			},
		},
	}, nil
}

func Test_injectCWSCommandInstrumentation(t *testing.T) {
	type args struct {
		exec     *corev1.PodExecOptions
		name     string
		ns       string
		userInfo *authenticationv1.UserInfo

		// configuration
		include            []string
		exclude            []string
		serviceAccountName string

		// mocked API client
		apiClientAnnotations map[string]string
		apiClientShouldFail  bool

		// mode
		cwsInstrumentationMode InstrumentationMode
	}
	mockConfig := configmock.New(t)
	tests := []struct {
		name string
		args args

		wantErr             bool
		wantInstrumentation bool
		wantPartialUserInfo bool
	}{
		{
			name: "CWS instrumentation ready, command, all namespaces",
			args: args{
				exec: &corev1.PodExecOptions{
					Command: []string{"bash"},
				},
				name:     "my-pod",
				ns:       "my-namespace",
				userInfo: &authenticationv1.UserInfo{},
				include:  []string{"kube_namespace:.*"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				},
				cwsInstrumentationMode: InitContainer,
			},
			wantInstrumentation: true,
		},
		{
			name: "CWS instrumentation ready, command, all namespaces, exclude container name",
			args: args{
				exec: &corev1.PodExecOptions{
					Command:   []string{"bash"},
					Container: "system-probe",
				},
				name:     "my-pod",
				ns:       "my-namespace",
				userInfo: &authenticationv1.UserInfo{},
				include:  []string{"kube_namespace:.*"},
				exclude:  []string{"name:system-probe"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				},
				cwsInstrumentationMode: InitContainer,
			},
			wantInstrumentation: true,
		},
		{
			name: "CWS instrumentation ready, command, all namespaces, include container name",
			args: args{
				exec: &corev1.PodExecOptions{
					Command:   []string{"bash"},
					Container: "system-probe",
				},
				name:     "my-pod",
				ns:       "my-namespace",
				userInfo: &authenticationv1.UserInfo{},
				include:  []string{"kube_namespace:.*", "name:system-probe"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				},
				cwsInstrumentationMode: InitContainer,
			},
			wantInstrumentation: true,
		},
		{
			name: "CWS instrumentation ready, command, no namespace",
			args: args{
				exec: &corev1.PodExecOptions{
					Command: []string{"bash"},
				},
				name:     "my-pod",
				ns:       "my-namespace",
				userInfo: &authenticationv1.UserInfo{},
				exclude:  []string{"kube_namespace:.*"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				},
				cwsInstrumentationMode: InitContainer,
			},
			wantInstrumentation: false,
		},
		{
			name: "CWS instrumentation ready, command, my namespace",
			args: args{
				exec: &corev1.PodExecOptions{
					Command: []string{"bash"},
				},
				name:     "my-pod",
				ns:       "my-namespace",
				userInfo: &authenticationv1.UserInfo{},
				include:  []string{"kube_namespae:my-namespace"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				},
				cwsInstrumentationMode: InitContainer,
			},
			wantInstrumentation: true,
		},
		{
			name: "CWS instrumentation ready, command, not my namespace",
			args: args{
				exec: &corev1.PodExecOptions{
					Command: []string{"bash"},
				},
				name:     "my-pod",
				ns:       "my-namespace",
				userInfo: &authenticationv1.UserInfo{},
				include:  []string{"kube_namespace:my-namespace2"},
				exclude:  []string{"kube_namespace:.*"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				},
				cwsInstrumentationMode: InitContainer,
			},
			wantInstrumentation: false,
		},
		{
			name: "CWS instrumentation not ready, command, all namespaces",
			args: args{
				exec: &corev1.PodExecOptions{
					Command: []string{"bash"},
				},
				name:     "my-pod",
				ns:       "my-namespace",
				userInfo: &authenticationv1.UserInfo{},
				include:  []string{"kube_namespace:.*"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: "hello",
				},
				cwsInstrumentationMode: InitContainer,
			},
			wantInstrumentation: false,
		},
		{
			name: "CWS instrumentation not present, command, all namespaces",
			args: args{
				exec: &corev1.PodExecOptions{
					Command: []string{"bash"},
				},
				name:                   "my-pod",
				ns:                     "my-namespace",
				userInfo:               &authenticationv1.UserInfo{},
				include:                []string{"kube_namespace:.*"},
				apiClientAnnotations:   map[string]string{},
				cwsInstrumentationMode: InitContainer,
			},
			wantInstrumentation: false,
		},
		{
			name: "CWS instrumentation ready, no command, all namespaces",
			args: args{
				exec: &corev1.PodExecOptions{
					Command: []string{},
				},
				name:     "my-pod",
				ns:       "my-namespace",
				userInfo: &authenticationv1.UserInfo{},
				include:  []string{"kube_namespace:.*"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				},
				cwsInstrumentationMode: InitContainer,
			},
			wantErr:             true,
			wantInstrumentation: false,
		},
		{
			name: "CWS instrumentation ready, empty exec, all namespaces",
			args: args{
				exec:     nil,
				name:     "my-pod",
				ns:       "my-namespace",
				userInfo: &authenticationv1.UserInfo{},
				include:  []string{"kube_namespace:.*"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				},
				cwsInstrumentationMode: InitContainer,
			},
			wantErr:             true,
			wantInstrumentation: false,
		},
		{
			name: "CWS instrumentation ready, empty user info, all namespaces",
			args: args{
				exec: &corev1.PodExecOptions{
					Command: []string{"bash"},
				},
				name:     "my-pod",
				ns:       "my-namespace",
				userInfo: nil,
				include:  []string{"kube_namespace:.*"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				},
				cwsInstrumentationMode: InitContainer,
			},
			wantErr:             true,
			wantInstrumentation: false,
		},
		{
			name: "CWS instrumentation ready, empty user info and exec, all namespaces",
			args: args{
				exec:     nil,
				name:     "my-pod",
				ns:       "my-namespace",
				userInfo: nil,
				include:  []string{"kube_namespace:.*"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				},
				cwsInstrumentationMode: InitContainer,
			},
			wantErr:             true,
			wantInstrumentation: false,
		},
		{
			name: "CWS instrumentation ready, command, all namespaces, failed API Client request",
			args: args{
				exec: &corev1.PodExecOptions{
					Command: []string{"bash"},
				},
				name:     "my-pod",
				ns:       "my-namespace",
				userInfo: &authenticationv1.UserInfo{},
				include:  []string{"kube_namespace:.*"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				},
				apiClientShouldFail:    true,
				cwsInstrumentationMode: InitContainer,
			},
			wantErr:             true,
			wantInstrumentation: false,
		},
		{
			name: "CWS instrumentation ready, command, all namespaces, failed user info serialization",
			args: args{
				exec: &corev1.PodExecOptions{
					Command: []string{"bash"},
				},
				name: "my-pod",
				ns:   "my-namespace",
				// a simple way to make the serialization fail is to make the username too big
				userInfo: &authenticationv1.UserInfo{
					Username: strings.Repeat("a", cwsUserSessionDataMaxSize+1),
				},
				include: []string{"kube_namespace:.*"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				},
				cwsInstrumentationMode: InitContainer,
			},
			wantErr:             true,
			wantInstrumentation: false,
		},
		{
			name: "CWS instrumentation ready, user session injected, all namespaces",
			args: args{
				exec: &corev1.PodExecOptions{
					Command: []string{
						filepath.Join(cwsMountPath, "cws-instrumentation"),
						"inject",
						"--session-type",
						"k8s",
						"--data",
						"{\"username\":\"hello\"}",
						"--",
						"bash",
					},
				},
				name: "my-pod",
				ns:   "my-namespace",
				userInfo: &authenticationv1.UserInfo{
					Username: "hello",
				},
				include: []string{"kube_namespace:.*"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				},
				cwsInstrumentationMode: InitContainer,
			},
			// we don't want to inject the user context twice if it is correct
			wantInstrumentation: false,
		},
		{
			name: "CWS instrumentation ready, wrong user session injected, all namespaces",
			args: args{
				exec: &corev1.PodExecOptions{
					Command: []string{
						filepath.Join(cwsMountPath, "cws-instrumentation"),
						"inject",
						"--session-type",
						"k8s",
						"--data",
						"{\"username\":\"bonsoir\"}",
						"--",
						"bash",
					},
				},
				name: "my-pod",
				ns:   "my-namespace",
				userInfo: &authenticationv1.UserInfo{
					Username: "hello",
				},
				include: []string{"kube_namespace:.*"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				},
				cwsInstrumentationMode: InitContainer,
			},
			// we will inject the user session context again, and rely on the CWS agent to dismiss the second injection attempt
			wantInstrumentation: true,
		},
		{
			name: "CWS instrumentation ready, command, all namespaces, service account filter",
			args: args{
				exec: &corev1.PodExecOptions{
					Container: "my-container",
					Command:   []string{"bash"},
				},
				name: "my-container",
				ns:   "default",
				userInfo: &authenticationv1.UserInfo{
					Username: "system:serviceaccount:default:datadog-cluster-agent",
				},
				include: []string{"kube_namespace:.*"},
				apiClientAnnotations: map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				},
				cwsInstrumentationMode: RemoteCopy,
				serviceAccountName:     "datadog-cluster-agent",
			},
			wantErr:             true,
			wantInstrumentation: false,
		},
	}

	// prepare the workload meta
	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig.SetWithoutSource("admission_controller.cws_instrumentation.include", tt.args.include)
			mockConfig.SetWithoutSource("admission_controller.cws_instrumentation.exclude", tt.args.exclude)
			mockConfig.SetWithoutSource("admission_controller.cws_instrumentation.mode", string(tt.args.cwsInstrumentationMode))
			mockConfig.SetWithoutSource("cluster_agent.service_account_name", tt.args.serviceAccountName)
			mockConfig.SetWithoutSource("kube_resources_namespace", tt.args.ns)

			var initialCommand string
			if tt.args.exec != nil {
				initialCommand = strings.Join(tt.args.exec.Command, " ")
			}

			ci, err := NewCWSInstrumentation(wmeta, mockConfig)
			if err != nil {
				require.Fail(t, "couldn't instantiate CWS Instrumentation", "%v", err)
			} else {
				apiClient := MockK8sClientInterface{
					annotations: tt.args.apiClientAnnotations,
					shouldFail:  tt.args.apiClientShouldFail,
				}
				if tt.args.exec != nil {
					apiClient.containerName = tt.args.exec.Container
				}
				injected, err := ci.injectCWSCommandInstrumentation(tt.args.exec, tt.args.name, tt.args.ns, tt.args.userInfo, nil, apiClient)

				if tt.wantErr {
					assert.False(t, injected)
				}

				if err != nil && !tt.wantErr {
					require.Fail(t, "CWS instrumentation shouldn't have produced an error: got %v", err)
				}

				if tt.wantInstrumentation {
					// make sure the cws-instrumentation command was injected
					if l := len(tt.args.exec.Command); l <= 7 {
						require.Fail(t, "CWS instrumentation failed", "invalid exec command length %d", l)
						return
					}
					assert.True(t, injected)
					expectedCommand := fmt.Sprintf("%s%s", filepath.Join(cwsMountPath, "cws-instrumentation"), " inject --session-type k8s --data")
					require.Equal(t, expectedCommand, strings.Join(tt.args.exec.Command[0:5], " "), "incorrect CWS instrumentation")
					require.Equal(t, "--", tt.args.exec.Command[6], "incorrect CWS instrumentation")
					require.LessOrEqual(t, len(tt.args.exec.Command[5]), cwsUserSessionDataMaxSize, "user session context too long")

					// check user session context
					var ui authenticationv1.UserInfo
					marshalErr := json.Unmarshal([]byte(tt.args.exec.Command[5]), &ui)
					require.Nil(t, marshalErr, "couldn't unmarshal injected UserInfo")
					if marshalErr == nil {
						if tt.wantPartialUserInfo {
							require.NotEqualf(t, *tt.args.userInfo, ui, "incorrect user session context")
						} else {
							require.Equal(t, *tt.args.userInfo, ui, "incorrect user session context")
						}
					}
				} else if tt.args.exec != nil {
					require.Equal(t, initialCommand, strings.Join(tt.args.exec.Command, " "), "CWS instrumentation shouldn't have modified the command")
				}
			}
		})
	}
}

func Test_injectCWSPodInstrumentation(t *testing.T) {
	commonRegistry := "gcr.io/datadoghq"

	type args struct {
		pod *corev1.Pod
		ns  string

		// configuration
		include []string
		exclude []string

		cwsInjectorImageName                string
		cwsInjectorImageTag                 string
		cwsInjectorContainerRegistry        string
		cwsInjectorMode                     InstrumentationMode
		cwsInjectorMountVolumeForRemoteCopy bool
		cwsInjectorServiceAccountName       string
	}
	tests := []struct {
		name                  string
		args                  args
		expectedInitContainer corev1.Container

		wantInstrumentation bool
		wantErr             bool
	}{
		{
			name: "all namespaces, image name, empty pod",
			args: args{
				pod:                          nil,
				ns:                           "my-namespace",
				include:                      []string{"kube_namespace:.*"},
				cwsInjectorImageName:         "my-image",
				cwsInjectorImageTag:          "",
				cwsInjectorContainerRegistry: "",
				cwsInjectorMode:              InitContainer,
			},
			wantErr: true,
		},
		{
			name: "all namespaces, image name",
			args: args{
				pod:                          common.FakePod("my-pod"),
				ns:                           "my-namespace",
				include:                      []string{"kube_namespace:.*"},
				cwsInjectorImageName:         "my-image",
				cwsInjectorImageTag:          "",
				cwsInjectorContainerRegistry: "",
				cwsInjectorMode:              InitContainer,
			},
			expectedInitContainer: corev1.Container{
				Name:    cwsInjectorInitContainerName,
				Image:   fmt.Sprintf("%s/my-image:latest", commonRegistry),
				Command: []string{"/cws-instrumentation", "setup", "--cws-volume-mount", cwsMountPath},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      cwsVolumeName,
						MountPath: cwsMountPath,
					},
				},
			},
			wantInstrumentation: true,
		},
		{
			name: "all namespaces, image name, image tag",
			args: args{
				pod:                          common.FakePod("my-pod"),
				ns:                           "my-namespace",
				include:                      []string{"kube_namespace:.*"},
				cwsInjectorImageName:         "my-image",
				cwsInjectorImageTag:          "my-tag",
				cwsInjectorContainerRegistry: "",
				cwsInjectorMode:              InitContainer,
			},
			expectedInitContainer: corev1.Container{
				Name:    cwsInjectorInitContainerName,
				Image:   fmt.Sprintf("%s/my-image:my-tag", commonRegistry),
				Command: []string{"/cws-instrumentation", "setup", "--cws-volume-mount", cwsMountPath},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      cwsVolumeName,
						MountPath: cwsMountPath,
					},
				},
			},
			wantInstrumentation: true,
		},
		{
			name: "all namespaces, image name, image tag, image registry",
			args: args{
				pod:                          common.FakePod("my-pod"),
				ns:                           "my-namespace",
				include:                      []string{"kube_namespace:.*"},
				cwsInjectorImageName:         "my-image",
				cwsInjectorImageTag:          "my-tag",
				cwsInjectorContainerRegistry: "my-registry",
				cwsInjectorMode:              InitContainer,
			},
			expectedInitContainer: corev1.Container{
				Name:    cwsInjectorInitContainerName,
				Image:   "my-registry/my-image:my-tag",
				Command: []string{"/cws-instrumentation", "setup", "--cws-volume-mount", cwsMountPath},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      cwsVolumeName,
						MountPath: cwsMountPath,
					},
				},
			},
			wantInstrumentation: true,
		},
		{
			name: "no namespace, image name",
			args: args{
				pod:                          common.FakePod("my-pod"),
				ns:                           "my-namespace",
				exclude:                      []string{"kube_namespace:.*"},
				cwsInjectorImageName:         "my-image",
				cwsInjectorImageTag:          "",
				cwsInjectorContainerRegistry: "",
				cwsInjectorMode:              InitContainer,
			},
		},
		{
			name: "my-namespace, image name",
			args: args{
				pod:                          common.FakePod("my-pod"),
				ns:                           "my-namespace",
				include:                      []string{"kube_namespace:my-namespace"},
				cwsInjectorImageName:         "my-image",
				cwsInjectorImageTag:          "",
				cwsInjectorContainerRegistry: "",
				cwsInjectorMode:              InitContainer,
			},
			expectedInitContainer: corev1.Container{
				Name:    cwsInjectorInitContainerName,
				Image:   fmt.Sprintf("%s/my-image:latest", commonRegistry),
				Command: []string{"/cws-instrumentation", "setup", "--cws-volume-mount", cwsMountPath},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      cwsVolumeName,
						MountPath: cwsMountPath,
					},
				},
			},
			wantInstrumentation: true,
		},
		{
			name: "missing my-namespace, image name",
			args: args{
				pod:                          common.FakePod("my-pod"),
				ns:                           "my-namespace",
				include:                      []string{"kube_namespace:my-namespace2"},
				exclude:                      []string{"kube_namespace:.*"},
				cwsInjectorImageName:         "my-image",
				cwsInjectorImageTag:          "",
				cwsInjectorContainerRegistry: "",
				cwsInjectorMode:              InitContainer,
			},
		},
		{
			name: "my-namespace skipped, image name",
			args: args{
				pod:                          common.FakePod("my-pod"),
				ns:                           "my-namespace",
				exclude:                      []string{"kube_namespace:my-namespace"},
				cwsInjectorImageName:         "my-image",
				cwsInjectorImageTag:          "",
				cwsInjectorContainerRegistry: "",
				cwsInjectorMode:              InitContainer,
			},
			wantInstrumentation: false,
		},
		{
			name: "my-namespace skipped, all namespaces, image name",
			args: args{
				pod:                          common.FakePod("my-pod"),
				ns:                           "my-namespace",
				exclude:                      []string{"kube_namespace:my-namespace"},
				cwsInjectorImageName:         "my-image",
				cwsInjectorImageTag:          "",
				cwsInjectorContainerRegistry: "",
				cwsInjectorMode:              InitContainer,
			},
			wantInstrumentation: false,
		},
		{
			name: "my-namespace skipped and selected, image name",
			args: args{
				pod:                          common.FakePod("my-pod"),
				ns:                           "my-namespace",
				include:                      []string{"kube_namespace:my-namespace"},
				exclude:                      []string{"kube_namespace:my-namespace"},
				cwsInjectorImageName:         "my-image",
				cwsInjectorImageTag:          "",
				cwsInjectorContainerRegistry: "",
				cwsInjectorMode:              InitContainer,
			},
			expectedInitContainer: corev1.Container{
				Name:    cwsInjectorInitContainerName,
				Image:   fmt.Sprintf("%s/my-image:latest", commonRegistry),
				Command: []string{"/cws-instrumentation", "setup", "--cws-volume-mount", cwsMountPath},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      cwsVolumeName,
						MountPath: cwsMountPath,
					},
				},
			},
			wantInstrumentation: true,
		},
		{
			name: "all namespaces, image name, CWS instrumentation ready",
			args: args{
				pod: common.FakePodWithAnnotations(map[string]string{
					cwsInstrumentationPodAnotationStatus: cwsInstrumentationPodAnotationReady,
				}),
				ns:                           "my-namespace",
				include:                      []string{"kube_namespace:.*"},
				cwsInjectorImageName:         "my-image",
				cwsInjectorImageTag:          "",
				cwsInjectorContainerRegistry: "",
				cwsInjectorMode:              InitContainer,
			},
			// when the annotation cwsInstrumentationPodAnotationStatus is set, the CWS mutating webhook should return without change the pod
			wantInstrumentation: false,
		},
		{
			name: "all namespaces, image name, CWS instrumentation",
			args: args{
				pod: common.FakePodWithAnnotations(map[string]string{
					cwsInstrumentationPodAnotationStatus: "hello",
				}),
				ns:                           "my-namespace",
				include:                      []string{"kube_namespace:.*"},
				cwsInjectorImageName:         "my-image",
				cwsInjectorImageTag:          "",
				cwsInjectorContainerRegistry: "",
				cwsInjectorMode:              InitContainer,
			},
			expectedInitContainer: corev1.Container{
				Name:    cwsInjectorInitContainerName,
				Image:   fmt.Sprintf("%s/my-image:latest", commonRegistry),
				Command: []string{"/cws-instrumentation", "setup", "--cws-volume-mount", cwsMountPath},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      cwsVolumeName,
						MountPath: cwsMountPath,
					},
				},
			},
			wantInstrumentation: true,
		},
		{
			name: "all namespaces, image name, remote copy, no volume",
			args: args{
				pod:                                 common.FakePod("my-pod"),
				ns:                                  "my-namespace",
				include:                             []string{"kube_namespace:.*"},
				cwsInjectorImageName:                "my-image",
				cwsInjectorImageTag:                 "",
				cwsInjectorContainerRegistry:        "",
				cwsInjectorMode:                     RemoteCopy,
				cwsInjectorMountVolumeForRemoteCopy: false,
				cwsInjectorServiceAccountName:       "datadog-cluster-agent",
			},
			wantInstrumentation: true,
		},
		{
			name: "all namespaces, image name, remote copy, no volume",
			args: args{
				pod:                                 common.FakePod("my-pod"),
				ns:                                  "my-namespace",
				include:                             []string{"kube_namespace:.*"},
				cwsInjectorImageName:                "my-image",
				cwsInjectorImageTag:                 "",
				cwsInjectorContainerRegistry:        "",
				cwsInjectorMode:                     RemoteCopy,
				cwsInjectorMountVolumeForRemoteCopy: true,
				cwsInjectorServiceAccountName:       "datadog-cluster-agent",
			},
			wantInstrumentation: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// prepare the workload meta
			wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))

			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("admission_controller.cws_instrumentation.include", tt.args.include)
			mockConfig.SetWithoutSource("admission_controller.cws_instrumentation.exclude", tt.args.exclude)
			mockConfig.SetWithoutSource("admission_controller.cws_instrumentation.image_name", tt.args.cwsInjectorImageName)
			mockConfig.SetWithoutSource("admission_controller.cws_instrumentation.image_tag", tt.args.cwsInjectorImageTag)
			mockConfig.SetWithoutSource("admission_controller.container_registry", commonRegistry)
			if tt.args.cwsInjectorContainerRegistry != "" {
				mockConfig.SetWithoutSource("admission_controller.cws_instrumentation.container_registry", tt.args.cwsInjectorContainerRegistry)
			}
			mockConfig.SetWithoutSource("admission_controller.cws_instrumentation.init_resources.cpu", "")
			mockConfig.SetWithoutSource("admission_controller.cws_instrumentation.init_resources.memory", "")
			mockConfig.SetWithoutSource("admission_controller.cws_instrumentation.mode", string(tt.args.cwsInjectorMode))
			mockConfig.SetWithoutSource("admission_controller.cws_instrumentation.remote_copy.mount_volume", tt.args.cwsInjectorMountVolumeForRemoteCopy)
			mockConfig.SetWithoutSource("cluster_agent.service_account_name", tt.args.cwsInjectorServiceAccountName)

			ci, err := NewCWSInstrumentation(wmeta, mockConfig)
			if err != nil {
				require.Fail(t, "couldn't instantiate CWS Instrumentation", "%v", err)
			} else {
				injected, err := ci.injectCWSPodInstrumentation(tt.args.pod, tt.args.ns, nil)

				if tt.wantErr {
					assert.False(t, injected)
				}

				if err != nil && !tt.wantErr {
					require.Fail(t, "CWS instrumentation shouldn't have produced an error: got %v", err)
				}

				if tt.wantInstrumentation {
					assert.True(t, injected)
					if tt.args.cwsInjectorMode == InitContainer || (tt.args.cwsInjectorMode == RemoteCopy && tt.args.cwsInjectorMountVolumeForRemoteCopy) {
						testInjectCWSVolume(t, tt.args.pod)
						testInjectCWSVolumeMount(t, tt.args.pod)

						if tt.args.cwsInjectorMode == InitContainer {
							testInjectCWSInitContainer(t, tt.args.pod, tt.expectedInitContainer)
						}

						// check annotation
						annotations := tt.args.pod.GetAnnotations()
						require.NotNil(t, annotations, "failed to annotate pod")
						if annotations != nil {
							require.Equal(t, cwsInstrumentationPodAnotationReady, annotations[cwsInstrumentationPodAnotationStatus], "CWS instrumentation annotation is missing")
							require.Equal(t, cwsVolumeName, annotations[common.K8sAutoscalerSafeToEvictVolumesAnnotation], "CWS instrumentation volume should be marked as safe to evict")
						}
					} else {
						testNoInjectedCWSVolume(t, tt.args.pod)
						testNoInjectedCWSVolumeMount(t, tt.args.pod)

						// check annotation
						annotations := tt.args.pod.GetAnnotations()
						if annotations != nil && tt.name != "all namespaces, image name, CWS instrumentation ready" {
							require.Emptyf(t, annotations[cwsInstrumentationPodAnotationStatus], "CWS instrumentation annotation should be missing")
						}
					}
				} else if tt.args.pod != nil {
					testNoInjectedCWSVolume(t, tt.args.pod)
					testNoInjectedCWSVolumeMount(t, tt.args.pod)
					testNoInjectedCWSInitContainer(t, tt.args.pod)

					// check annotation
					annotations := tt.args.pod.GetAnnotations()
					if annotations != nil && tt.name != "all namespaces, image name, CWS instrumentation ready" {
						require.Emptyf(t, annotations[cwsInstrumentationPodAnotationStatus], "CWS instrumentation annotation should be missing")
					}
				}
			}
		})
	}
}

// testInjectCWSVolume checks if the CWS volume was properly injected
func testInjectCWSVolume(t *testing.T, pod *corev1.Pod) {
	for _, vol := range pod.Spec.Volumes {
		if vol.Name == cwsVolumeName {
			require.NotNil(t, vol.EmptyDir, "CWS instrumentation volume should be and empty directory")
			return
		}
	}
	require.Fail(t, "CWS instrumentation volume \"%s\" is missing", cwsVolumeName)
}

// testNoInjectedCWSVolume checks that a CWS instrumentation volume wasn't injected
func testNoInjectedCWSVolume(t *testing.T, pod *corev1.Pod) {
	for _, vol := range pod.Spec.Volumes {
		require.NotEqualf(t, cwsVolumeName, vol.Name, "CWS instrumentation should be missing")
	}
}

// testInjectCWSVolumeMount checks if the CWS volume mount was properly injected in all containers
func testInjectCWSVolumeMount(t *testing.T, pod *corev1.Pod) {
mainLoop:
	for _, container := range pod.Spec.Containers {
		// for each container, look for the CWS instrumentation volume mount
		for _, mnt := range container.VolumeMounts {
			if mnt.Name == cwsVolumeName {
				require.Equal(t, cwsMountPath, mnt.MountPath, "Container [%s] has an incorrect CWS volume mount path")
				continue mainLoop
			}
		}
		require.Fail(t, "Container [%s] is missing the CWS instrumentation volume mount")
	}
}

// testNoInjectedCWSVolumeMount checks that no CWS volume mount was injected
func testNoInjectedCWSVolumeMount(t *testing.T, pod *corev1.Pod) {
	for _, container := range pod.Spec.Containers {
		for _, mnt := range container.VolumeMounts {
			require.NotEqualf(t, cwsVolumeName, mnt.Name, "CWS instrumentation should be missing")
		}
	}
}

// testInjectCWSInitContainer checks if the CWS instrumentation init container was properly injected
func testInjectCWSInitContainer(t *testing.T, pod *corev1.Pod, initContainer corev1.Container) {
	var container *corev1.Container
	for _, c := range pod.Spec.InitContainers {
		if c.Name == cwsInjectorInitContainerName {
			container = &c
		}
	}
	if container == nil {
		require.Fail(t, "CWS instrumentation init container is missing")
		return
	}

	// check the init container itself
	require.Equal(t, *container, initContainer, "incorrect CWS instrumentation init container")
}

// testNoInjectedCWSInitContainer checks that no CWS instrumentation init container was injected
func testNoInjectedCWSInitContainer(t *testing.T, pod *corev1.Pod) {
	for _, c := range pod.Spec.InitContainers {
		require.NotEqualf(t, cwsInjectorInitContainerName, c.Name, "CWS instrumentation should be missing")
	}
}

func Test_initCWSInitContainerResources(t *testing.T) {
	mockConfig := configmock.New(t)
	tests := []struct {
		name       string
		mem        string
		cpu        string
		wantMemory bool
		wantCPU    bool
		wantErr    bool
	}{
		{
			name: "empty resources",
			mem:  "",
			cpu:  "",
		},
		{
			name:       "mem",
			mem:        "100Mi",
			cpu:        "",
			wantMemory: true,
		},
		{
			name:    "cpu",
			mem:     "",
			cpu:     "100m",
			wantCPU: true,
		},
		{
			name:       "mem, cpu",
			mem:        "10Gi",
			cpu:        "2",
			wantCPU:    true,
			wantMemory: true,
		},
		{
			name:    "invalid mem",
			mem:     "abc",
			cpu:     "",
			wantErr: true,
		},
		{
			name:    "invalid cpu",
			mem:     "",
			cpu:     "abc",
			wantErr: true,
		},
		{
			name:    "invalid mem, invalid cpu",
			mem:     "abc",
			cpu:     "abc",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig.SetWithoutSource("admission_controller.cws_instrumentation.init_resources.cpu", tt.cpu)
			mockConfig.SetWithoutSource("admission_controller.cws_instrumentation.init_resources.memory", tt.mem)

			got, err := parseCWSInitContainerResources()
			if err != nil && !tt.wantErr {
				require.Nil(t, err, "an error shouldn't have been generated")
			}

			if tt.wantCPU == false && tt.wantMemory == false {
				require.Nil(t, got, "initCWSInitContainerResources() shouldn't return resources")
				return
			}

			memReq, okMemReq := got.Requests[corev1.ResourceMemory]
			memLim, okMemLim := got.Requests[corev1.ResourceMemory]
			if tt.wantMemory {
				require.Equal(t, tt.mem, memReq.String(), "initCWSInitContainerResources(), invalid memory requests")
				require.Equal(t, tt.mem, memLim.String(), "initCWSInitContainerResources(), invalid memory limits")
			} else {
				if okMemReq && okMemLim {
					require.Nil(t, memReq, "initCWSInitContainerResources(), memory requests should be nil")
					require.Nil(t, memLim, "initCWSInitContainerResources(), memory limits should be nil")
				}
			}

			cpuReq, okCPUReq := got.Requests[corev1.ResourceCPU]
			cpuLim, okCPULim := got.Requests[corev1.ResourceCPU]
			if tt.wantCPU {
				require.Equal(t, tt.cpu, cpuReq.String(), "initCWSInitContainerResources(), invalid cpu requests")
				require.Equal(t, tt.cpu, cpuLim.String(), "initCWSInitContainerResources(), invalid cpu limits")
			} else {
				if okCPUReq && okCPULim {
					require.Nil(t, cpuReq, "initCWSInitContainerResources(), cpu requests should be nil")
					require.Nil(t, cpuLim, "initCWSInitContainerResources(), cpu limits should be nil")
				}
			}
		})
	}
}

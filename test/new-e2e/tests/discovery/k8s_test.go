// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package discovery

import (
	"bytes"
	"context"
	_ "embed"
	"testing"
	"text/template"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/discovery"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/process"
	localkubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes"
)

//go:embed config/helm-values.tmpl
var helmValuesTemplate string

type helmConfig struct {
	UseSystemProbeLite bool
}

func createHelmValues(cfg helmConfig) (string, error) {
	var buffer bytes.Buffer
	tmpl, err := template.New("helm").Parse(helmValuesTemplate)
	if err != nil {
		return "", err
	}
	err = tmpl.Execute(&buffer, cfg)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
}

type k8sTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// k8sProvisioner returns the provisioner for the K8s discovery tests.
// Switch between localkubernetes.Provisioner (local dev) and
// provkindvm.Provisioner (CI) by commenting/uncommenting below.
func k8sProvisioner(helmValues string) provisioners.TypedProvisioner[environments.Kubernetes] {
	// Local development: use local Kind cluster
	return localkubernetes.Provisioner(
		localkubernetes.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
			return discovery.K8sAppDefinition(e, kubeProvider, "discovery-workloads")
		}),
		localkubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(helmValues)),
	)
	// CI: use AWS KindVM
	// return provkindvm.Provisioner(
	// 	provkindvm.WithRunOptions(
	// 		scenkindvm.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
	// 			return discovery.K8sAppDefinition(e, kubeProvider, "discovery-workloads")
	// 		}),
	// 		scenkindvm.WithAgentOptions(kubernetesagentparams.WithHelmValues(helmValues)),
	// 	),
	// )
}

func TestK8sTestSuite(t *testing.T) {
	t.Parallel()

	helmValues, err := createHelmValues(helmConfig{UseSystemProbeLite: true})
	require.NoError(t, err)

	e2e.Run(t, &k8sTestSuite{},
		e2e.WithStackName("discovery"),
		e2e.WithProvisioner(k8sProvisioner(helmValues)),
	)
}

func (s *k8sTestSuite) TestProcessCheckWithServiceDiscovery() {
	for _, mode := range []discoveryMode{discoveryModeSystemProbeLite, discoveryModeSystemProbe} {
		s.Run(string(mode), func() {
			useSystemProbeLite := mode == discoveryModeSystemProbeLite
			helmValues, err := createHelmValues(helmConfig{UseSystemProbeLite: useSystemProbeLite})
			require.NoError(s.T(), err)

			s.UpdateEnv(k8sProvisioner(helmValues))

			s.validateDiscoveryMode(mode)

			client := s.Env().FakeIntake.Client()
			err = client.FlushServerAndResetAggregators()
			require.NoError(s.T(), err)

			t := s.T()
			for _, tc := range []struct {
				description      string
				processName      string
				expectedLanguage agentmodel.Language
				expectedPortInfo *agentmodel.PortInfo
				expectedService  *agentmodel.ServiceDiscovery
			}{
				{
					description:      "node-json-server",
					processName:      "node",
					expectedLanguage: agentmodel.Language_LANGUAGE_NODE,
					expectedPortInfo: &agentmodel.PortInfo{
						Tcp: []int32{8084},
					},
					expectedService: &agentmodel.ServiceDiscovery{
						GeneratedServiceName: &agentmodel.ServiceName{
							Name:   "json-server",
							Source: agentmodel.ServiceNameSource_SERVICE_NAME_SOURCE_NODEJS,
						},
					},
				},
				{
					description:      "node-instrumented",
					processName:      "node",
					expectedLanguage: agentmodel.Language_LANGUAGE_NODE,
					expectedPortInfo: &agentmodel.PortInfo{
						Tcp: []int32{8085},
					},
					expectedService: &agentmodel.ServiceDiscovery{
						ApmInstrumentation: true,
						GeneratedServiceName: &agentmodel.ServiceName{
							Name:   "node-instrumented",
							Source: agentmodel.ServiceNameSource_SERVICE_NAME_SOURCE_NODEJS,
						},
						TracerMetadata: []*agentmodel.TracerMetadata{
							{
								ServiceName: "node-instrumented",
							},
						},
					},
				},
				{
					// Container base images use /usr/local/bin/python3 instead of /usr/bin/python3
					description:      "python-svc",
					processName:      "/usr/local/bin/python3",
					expectedLanguage: agentmodel.Language_LANGUAGE_PYTHON,
					expectedPortInfo: &agentmodel.PortInfo{
						Tcp: []int32{8082},
					},
					expectedService: &agentmodel.ServiceDiscovery{
						GeneratedServiceName: &agentmodel.ServiceName{
							Name:   "python.server",
							Source: agentmodel.ServiceNameSource_SERVICE_NAME_SOURCE_PYTHON,
						},
						DdServiceName: &agentmodel.ServiceName{
							Name:   "python-svc-dd",
							Source: agentmodel.ServiceNameSource_SERVICE_NAME_SOURCE_DD_SERVICE,
						},
					},
				},
				{
					description:      "python-instrumented",
					processName:      "/usr/local/bin/python3",
					expectedLanguage: agentmodel.Language_LANGUAGE_PYTHON,
					expectedPortInfo: &agentmodel.PortInfo{
						Tcp: []int32{8083},
					},
					expectedService: &agentmodel.ServiceDiscovery{
						ApmInstrumentation: true,
						GeneratedServiceName: &agentmodel.ServiceName{
							Name:   "python.instrumented",
							Source: agentmodel.ServiceNameSource_SERVICE_NAME_SOURCE_PYTHON,
						},
						DdServiceName: &agentmodel.ServiceName{
							Name:   "python-instrumented-dd",
							Source: agentmodel.ServiceNameSource_SERVICE_NAME_SOURCE_DD_SERVICE,
						},
						TracerMetadata: []*agentmodel.TracerMetadata{
							{
								ServiceName: "python-instrumented-dd",
							},
						},
					},
				},
				{
					// Container base images use "ruby" instead of "ruby3.0"
					description:      "rails-svc",
					processName:      "ruby",
					expectedLanguage: agentmodel.Language_LANGUAGE_RUBY,
					expectedPortInfo: &agentmodel.PortInfo{
						Tcp: []int32{7777},
					},
					expectedService: &agentmodel.ServiceDiscovery{
						GeneratedServiceName: &agentmodel.ServiceName{
							Name:   "rails_hello",
							Source: agentmodel.ServiceNameSource_SERVICE_NAME_SOURCE_RAILS,
						},
					},
				},
			} {
				ok := t.Run(tc.description, func(t *testing.T) {
					var payloads []*aggregator.ProcessPayload
					assert.EventuallyWithT(t, func(c *assert.CollectT) {
						var err error
						payloads, err = s.Env().FakeIntake.Client().GetProcesses()
						assert.NoError(c, err, "failed to get process payloads from fakeintake")
						assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")

						procs := process.FilterProcessPayloadsByName(payloads, tc.processName)
						assert.NotEmpty(c, procs, "'%s' process not found in payloads: \n%+v", tc.processName, payloads)
						assert.True(c, matchingProcessServiceDiscoveryData(procs, tc.expectedLanguage, tc.expectedPortInfo, tc.expectedService),
							"no process was found with the expected service discovery data. processes:\n%+v", procs)
					}, 5*time.Minute, 10*time.Second)
				})
				if !ok {
					s.dumpDebugInfo(t)
				}
			}
		})
	}
}

func (s *k8sTestSuite) validateDiscoveryMode(mode discoveryMode) {
	t := s.T()
	agentPod := s.getAgentPod(t)

	stdout, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
		agentPod.Namespace, agentPod.Name, "system-probe",
		[]string{"sh", "-c", "ps aux | grep system-probe | grep -v grep"})
	require.NoError(t, err, "failed to exec ps in system-probe container")
	t.Logf("Process list:\n%s", stdout)

	if mode == discoveryModeSystemProbeLite {
		require.Contains(t, stdout, "system-probe-lite", "system-probe-lite should be running in system-probe-lite mode")
		t.Logf("Found system-probe-lite process (mode: %s)", mode)
	} else if mode == discoveryModeSystemProbe {
		require.NotContains(t, stdout, "system-probe-lite", "system-probe-lite should not be running in system-probe mode")
		require.Contains(t, stdout, "system-probe", "system-probe should be running in system-probe mode")
		t.Logf("Found system-probe process (mode: %s)", mode)
	}
}

func (s *k8sTestSuite) dumpDebugInfo(t *testing.T) {
	agentPod := s.getAgentPod(t)

	stdout, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
		agentPod.Namespace, agentPod.Name, "system-probe",
		[]string{"curl", "-s", "--unix-socket", "/var/run/sysprobe/sysprobe.sock", "http://unix/discovery/debug"})
	if err == nil {
		t.Log("system-probe services", stdout)
	} else {
		t.Log("failed to get discovery debug info", err)
	}

	stdout, _, err = s.Env().KubernetesCluster.KubernetesClient.PodExec(
		agentPod.Namespace, agentPod.Name, "agent",
		[]string{"agent", "status"})
	if err == nil {
		t.Log("agent status", stdout)
	} else {
		t.Log("failed to get agent status", err)
	}
}

func (s *k8sTestSuite) getAgentPod(t testing.TB) corev1.Pod {
	res, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").
		List(context.Background(), v1.ListOptions{LabelSelector: "app=dda-linux-datadog"})
	require.NoError(t, err)
	require.NotEmpty(t, res.Items, "no agent pods found")

	// Select a pod with a ready system-probe container
	for _, pod := range res.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Name == "system-probe" && cs.Ready {
				return pod
			}
		}
	}

	// Fallback to any pod
	return res.Items[0]
}

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
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/process"
)

const (
	nginxNamespace = "workload-nginx"
	nginxPort      = 80
)

//go:embed config/helm-values.tmpl
var helmValuesTemplate string

type helmConfig struct {
	UseSystemProbeLite bool
}

func createHelmValues(cfg helmConfig) (string, error) {
	var buf bytes.Buffer
	tmpl, err := template.New("helm").Parse(helmValuesTemplate)
	if err != nil {
		return "", err
	}
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func k8sProvisioner(helmValues string) provisioners.TypedProvisioner[environments.Kubernetes] {
	return provkindvm.Provisioner(
		provkindvm.WithRunOptions(
			scenkindvm.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
				return nginx.K8sAppDefinition(e, kubeProvider, nginxNamespace, nginxPort, "", false, nil)
			}),
			scenkindvm.WithAgentOptions(kubernetesagentparams.WithHelmValues(helmValues)),
		),
	)
}

type k8sTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestK8sTestSuite(t *testing.T) {
	t.Parallel()

	helmValues, err := createHelmValues(helmConfig{UseSystemProbeLite: true})
	require.NoError(t, err)

	e2e.Run(t, &k8sTestSuite{}, e2e.WithProvisioner(k8sProvisioner(helmValues)))
}

func (s *k8sTestSuite) TestNginxDiscovered() {
	for _, mode := range []discoveryMode{discoveryModeSystemProbeLite, discoveryModeSystemProbe} {
		s.Run(string(mode), func() {
			useSystemProbeLite := mode == discoveryModeSystemProbeLite
			helmValues, err := createHelmValues(helmConfig{UseSystemProbeLite: useSystemProbeLite})
			require.NoError(s.T(), err)
			s.UpdateEnv(k8sProvisioner(helmValues))

			s.validateK8sDiscoveryMode(mode)

			require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

			t := s.T()
			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				payloads, err := s.Env().FakeIntake.Client().GetProcesses()
				assert.NoError(c, err, "failed to get process payloads from fakeintake")
				assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")

				procs := process.FilterProcessPayloadsByName(payloads, "nginx")
				assert.NotEmpty(c, procs, "no nginx process found in payloads")

				assert.True(c, anyProcessListensOnPort(procs, int32(nginxPort)),
					"no nginx process was reported listening on tcp/%d. processes: %+v", nginxPort, procs)
			}, 5*time.Minute, 10*time.Second)

			if t.Failed() {
				s.dumpK8sDebugInfo(t)
			}
		})
	}
}

// anyProcessListensOnPort returns true if any of the given processes reports
// a TCP listener on the specified port. The discovery sanity check only cares
// that the port is detected; we don't assert on language, service name, or
// tracer metadata (those are covered by the VM suite).
func anyProcessListensOnPort(procs []*agentmodel.Process, port int32) bool {
	for _, p := range procs {
		if p.PortInfo == nil {
			continue
		}
		for _, tcp := range p.PortInfo.Tcp {
			if tcp == port {
				return true
			}
		}
	}
	return false
}

func (s *k8sTestSuite) validateK8sDiscoveryMode(mode discoveryMode) {
	t := s.T()
	agentPod := s.getAgentPod(t)
	stdout, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
		agentPod.Namespace, agentPod.Name, "system-probe",
		[]string{"sh", "-c", "ps aux | grep system-probe | grep -v grep"})
	require.NoError(t, err, "failed to exec ps in system-probe container")
	t.Logf("Process list:\n%s", stdout)

	switch mode {
	case discoveryModeSystemProbeLite:
		require.Contains(t, stdout, "system-probe-lite", "system-probe-lite should be running in system-probe-lite mode")
	case discoveryModeSystemProbe:
		require.NotContains(t, stdout, "system-probe-lite", "system-probe-lite should not be running in system-probe mode")
		require.Contains(t, stdout, "system-probe", "system-probe should be running in system-probe mode")
	}
	t.Logf("Discovery mode confirmed: %s", mode)
}

func (s *k8sTestSuite) dumpK8sDebugInfo(t *testing.T) {
	agentPod := s.getAgentPod(t)
	if stdout, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
		agentPod.Namespace, agentPod.Name, "system-probe",
		[]string{"curl", "-s", "--unix-socket", "/var/run/sysprobe/sysprobe.sock", "http://unix/discovery/debug"}); err == nil {
		t.Log("system-probe discovery debug:\n", stdout)
	} else {
		t.Log("failed to get discovery debug info:", err)
	}
	if stdout, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
		agentPod.Namespace, agentPod.Name, "agent",
		[]string{"agent", "status"}); err == nil {
		t.Log("agent status:\n", stdout)
	} else {
		t.Log("failed to get agent status:", err)
	}
}

func (s *k8sTestSuite) getAgentPod(t testing.TB) corev1.Pod {
	res, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").
		List(context.Background(), v1.ListOptions{LabelSelector: "app=dda-linux-datadog"})
	require.NoError(t, err)
	require.NotEmpty(t, res.Items, "no agent pods found")
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
	return res.Items[0]
}

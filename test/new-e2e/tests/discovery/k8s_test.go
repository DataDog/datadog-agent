// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package discovery

import (
	"bytes"
	"context"
	_ "embed"
	"slices"
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
	// EnableOOMKill forces full system-probe to run by enabling a
	// second system-probe feature alongside discovery. The agent's
	// shouldExecSPLite logic only exec's into system-probe-lite when
	// discovery is the *only* enabled system-probe module
	// (cmd/system-probe/subcommands/run/splite.go).
	EnableOOMKill bool
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

	// Initial helm values: discovery only → SPL mode.
	helmValues, err := createHelmValues(helmConfig{})
	require.NoError(t, err)

	e2e.Run(t, &k8sTestSuite{}, e2e.WithProvisioner(k8sProvisioner(helmValues)))
}

func (s *k8sTestSuite) TestNginxDiscovered() {
	for _, mode := range []discoveryMode{discoveryModeSystemProbeLite, discoveryModeSystemProbe} {
		s.Run(string(mode), func() {
			helmValues, err := createHelmValues(helmConfig{
				EnableOOMKill: mode == discoveryModeSystemProbe,
			})
			require.NoError(s.T(), err)
			s.UpdateEnv(k8sProvisioner(helmValues))

			s.validateDiscoveryMode(mode)

			require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

			t := s.T()
			var matched *agentmodel.Process
			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				payloads, err := s.Env().FakeIntake.Client().GetProcesses()
				assert.NoError(c, err, "failed to get process payloads from fakeintake")
				assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")

				procs := process.FilterProcessPayloadsByName(payloads, "nginx")
				assert.NotEmpty(c, procs, "no nginx process found in payloads")

				for _, p := range procs {
					if p.PortInfo != nil && slices.Contains(p.PortInfo.Tcp, int32(nginxPort)) {
						matched = p
						break
					}
				}
				assert.NotNil(c, matched, "no nginx process was reported listening on tcp/%d", nginxPort)
			}, 5*time.Minute, 10*time.Second)

			// Log the matched payload so we can tighten assertions in a follow-up.
			if matched != nil {
				t.Logf("matched nginx process payload:\n%+v\nServiceDiscovery: %+v\nPortInfo: %+v",
					matched, matched.ServiceDiscovery, matched.PortInfo)
			}
		})
	}
}

// validateDiscoveryMode asserts the agent's system-probe container is running
// the expected binary (full system-probe or system-probe-lite). Wrapped in
// EventuallyWithT to tolerate the post-UpdateEnv pod-restart window where
// getAgentPod would otherwise fail with "no agent pod with ready system-probe
// container found".
func (s *k8sTestSuite) validateDiscoveryMode(mode discoveryMode) {
	t := s.T()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		agentPod, ok := tryFindReadyAgentPod(c, s)
		if !ok {
			return
		}
		stdout, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
			agentPod.Namespace, agentPod.Name, "system-probe",
			[]string{"sh", "-c", "ps aux | grep system-probe | grep -v grep"})
		assert.NoError(c, err, "failed to exec ps in system-probe container")
		t.Logf("Process list (mode=%s):\n%s", mode, stdout)
		switch mode {
		case discoveryModeSystemProbeLite:
			assert.Contains(c, stdout, "system-probe-lite", "system-probe-lite should be running in system-probe-lite mode")
		case discoveryModeSystemProbe:
			assert.NotContains(c, stdout, "system-probe-lite", "system-probe-lite should not be running in system-probe mode")
			assert.Contains(c, stdout, "system-probe", "system-probe should be running in system-probe mode")
		}
	}, 2*time.Minute, 5*time.Second)
}

// tryFindReadyAgentPod returns an agent pod with a Ready system-probe container,
// or (zeroPod, false) if none is currently ready. Designed for use inside
// EventuallyWithT, where "not yet" is a normal transient state, not a failure.
func tryFindReadyAgentPod(c *assert.CollectT, s *k8sTestSuite) (corev1.Pod, bool) {
	res, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").
		List(context.Background(), v1.ListOptions{LabelSelector: "app=dda-linux-datadog"})
	if !assert.NoError(c, err, "failed to list agent pods") {
		return corev1.Pod{}, false
	}
	if !assert.NotEmpty(c, res.Items, "no agent pods listed") {
		return corev1.Pod{}, false
	}
	for _, pod := range res.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Name == "system-probe" && cs.Ready {
				return pod, true
			}
		}
	}
	assert.Fail(c, "no agent pod has a ready system-probe container yet")
	return corev1.Pod{}, false
}

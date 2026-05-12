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
	// helmChartVersion needs to be ≥ 3.205.0 for the
	// discovery-use-system-probe-lite template (DataDog/helm-charts#2598)
	// and ≥ 3.213.0 for the get-agent-version fallback fix
	// (DataDog/helm-charts#2643) that maps "latest"/"7" to ≥ 7.78.0 so
	// the SPL auto-enable threshold is met without an explicit image
	// tag pin. The framework's default chart version predates both.
	helmChartVersion = "3.213.0"
)

//go:embed config/helm-values.tmpl
var helmValuesTemplate string

type helmConfig struct {
	// Mode controls which discovery mode the chart renders for, and
	// is also written verbatim as a DaemonSet pod annotation. The
	// annotation is the mechanism that actually rolls the pod when
	// we flip modes via UpdateEnv between sub-tests — without it,
	// only the system-probe configmap changes and the existing pod
	// stays alive (the chart's daemonset.yaml has no checksum
	// annotation tying the pod template to that configmap).
	//
	// "spl" → discovery alone → agent execs into system-probe-lite
	// "sp"  → discovery + datadog.systemProbe.enableOOMKill → the
	//         agent's shouldExecSPLite logic keeps the full binary
	//         running because discovery is no longer the only
	//         enabled system-probe module
	//         (cmd/system-probe/subcommands/run/splite.go).
	Mode string
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
			scenkindvm.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(helmValues),
				// Pin to a chart version that has the discovery /
				// system-probe-lite logic. The framework default
				// (HelmVersion in kubernetes_helm.go) predates it.
				kubernetesagentparams.WithHelmChartVersion(helmChartVersion),
			),
		),
	)
}

type k8sTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestK8sTestSuite(t *testing.T) {
	t.Parallel()

	// Initial helm values: SPL mode. The first sub-test will UpdateEnv
	// to the same mode (no-op) and the second to "sp".
	helmValues, err := createHelmValues(helmConfig{Mode: "spl"})
	require.NoError(t, err)

	e2e.Run(t, &k8sTestSuite{}, e2e.WithProvisioner(k8sProvisioner(helmValues)))
}

func (s *k8sTestSuite) TestNginxDiscovered() {
	for _, mode := range []discoveryMode{discoveryModeSystemProbeLite, discoveryModeSystemProbe} {
		s.Run(string(mode), func() {
			modeName := "spl"
			if mode == discoveryModeSystemProbe {
				modeName = "sp"
			}
			helmValues, err := createHelmValues(helmConfig{Mode: modeName})
			require.NoError(s.T(), err)
			s.UpdateEnv(k8sProvisioner(helmValues))

			s.validateDiscoveryMode(mode)

			require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

			t := s.T()
			var matched *agentmodel.Process
			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				matched = nil
				payloads, err := s.Env().FakeIntake.Client().GetProcesses()
				if !assert.NoError(c, err, "failed to get process payloads from fakeintake") {
					return
				}
				if !assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned") {
					return
				}

				procs := process.FilterProcessPayloadsByName(payloads, "nginx")
				if !assert.NotEmpty(c, procs, "no nginx process found in payloads") {
					return
				}

				for _, p := range procs {
					if p.PortInfo != nil && slices.Contains(p.PortInfo.Tcp, int32(nginxPort)) {
						matched = p
						break
					}
				}
				if !assert.NotNil(c, matched, "no nginx process was reported listening on tcp/%d", nginxPort) {
					return
				}

				// nginx is C/native, so no language or tracer-metadata
				// fields are populated. What discovery does fill in:
				sd := matched.ServiceDiscovery
				if !assert.NotNil(c, sd, "ServiceDiscovery is unset on the matched process") {
					return
				}
				if assert.NotNil(c, sd.GeneratedServiceName, "GeneratedServiceName is unset") {
					assert.Equal(c, "nginx", sd.GeneratedServiceName.Name, "generated service name should be 'nginx'")
					assert.Equal(c, agentmodel.ServiceNameSource_SERVICE_NAME_SOURCE_COMMAND_LINE, sd.GeneratedServiceName.Source,
						"service name source should be SERVICE_NAME_SOURCE_COMMAND_LINE")
				}
				assert.Equal(c, agentmodel.InjectionState_INJECTION_NOT_INJECTED, matched.InjectionState,
					"nginx isn't instrumented; injection state should be NOT_INJECTED")
				assert.False(c, sd.ApmInstrumentation, "ServiceDiscovery.ApmInstrumentation should be false for unmodified nginx")
				assert.NotEmpty(c, matched.ContainerId, "containerId should be populated by the discovery → tagger pipeline")
			}, 5*time.Minute, 10*time.Second)
			require.NotNil(t, matched, "no matching nginx process found")

			t.Logf("matched nginx process payload:\n%+v", matched)
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package discovery

import (
	_ "embed"
	"testing"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/process"
)

const (
	nginxNamespace = "workload-nginx"
	nginxPort      = 80
)

//go:embed config/helm-values.tmpl
var helmValues string

type k8sTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestK8sTestSuite(t *testing.T) {
	t.Parallel()

	e2e.Run(t, &k8sTestSuite{},
		e2e.WithProvisioner(
			provkindvm.Provisioner(
				provkindvm.WithRunOptions(
					scenkindvm.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
						return nginx.K8sAppDefinition(e, kubeProvider, nginxNamespace, nginxPort, "", false, nil)
					}),
					scenkindvm.WithAgentOptions(kubernetesagentparams.WithHelmValues(helmValues)),
				),
			),
		),
	)
}

// TestNginxDiscovered verifies that the agent's discovery feature reports
// the nginx workload to fakeintake. We don't differentiate between
// system-probe and system-probe-lite modes — this is a sanity check that
// the discovery flow works at all on k8s, in whichever mode the chart picks.
func (s *k8sTestSuite) TestNginxDiscovered() {
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

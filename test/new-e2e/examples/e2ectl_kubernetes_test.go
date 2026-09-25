// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	e2ectlenv "github.com/DataDog/datadog-agent/test/new-e2e/utils/e2ectlenv"
)

// The minimal example of a test that attaches to a Kubernetes environment
// provisioned and agent-installed with e2ectl. No provisioner is defined in
// the test: the environment comes from the config file next to this file
// (e2ectl-kubernetes.yml), and the entry point attaches to the live
// environment named by E2ECTL_ENV. The full workflow:
//
//	e2ectl start --config e2ectl-kubernetes.yml --name mykube
//	e2ectl install -env mykube --config e2ectl-kubernetes.yml
//	e2ectl test -env mykube --suite ./test/new-e2e/examples/
//
// The same test body runs against any Kubernetes base: on an EKS environment
// (examples: test/e2e-framework/cmd/e2ectl/examples/eks.yml) pass the pattern
// explicitly, e.g. --run '^TestE2ectlKubernetesSuiteOnLocalKind$'.

// e2ectlKubernetesSuite is the whole suite: the BaseSuite provides the typed
// environment accessors (Env(), FakeIntake, Agent) and nothing else.
type e2ectlKubernetesSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestE2ectlKubernetesSuite(t *testing.T) {
	envName := e2ectlenv.RequireEnv(t)
	e2ectlenv.RequireSnapshot(t, envName)
	t.Parallel()
	e2e.Run(t, &e2ectlKubernetesSuite{}, e2e.WithProvisioner(
		e2ectlenv.Attach[environments.Kubernetes](envName),
	))
}

// TestAgentRunning is the minimal end-to-end signal: the agent installed in
// the environment is alive and its metrics reach the receiver (fakeintake by
// default). Everything beyond this — workload metrics, filtering, logs — is
// the same pattern with a different metric name or payload endpoint.
func (s *e2ectlKubernetesSuite) TestAgentRunning() {
	client := s.Env().FakeIntake.Client()
	require.Eventually(s.T(), func() bool {
		names, err := client.GetMetricNames()
		return err == nil && slices.Contains(names, "datadog.agent.running")
	}, 5*time.Minute, 10*time.Second, "the datadog.agent.running metric should reach the fakeintake")
}

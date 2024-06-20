// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package process

import (
	"bytes"
	"context"
	_ "embed"
	"testing"
	"text/template"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/cpustress"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeClient "k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
)

// helmTemplate define the embedded minimal configuration for NPM
//
//go:embed config/helm-template.tmpl
var helmTemplate string

type helmConfig struct {
	ProcessAgentEnabled        bool
	ProcessCollection          bool
	ProcessDiscoveryCollection bool
}

func createHelmValues(cfg helmConfig) (string, error) {
	var buffer bytes.Buffer
	tmpl, err := template.New("agent").Parse(helmTemplate)
	if err != nil {
		return "", err
	}
	err = tmpl.Execute(&buffer, cfg)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
}

type K8sSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestK8sTestSuite(t *testing.T) {
	helmValues, err := createHelmValues(helmConfig{
		ProcessAgentEnabled: true,
		ProcessCollection:   true,
	})
	require.NoError(t, err)

	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awskubernetes.KindProvisioner(
			awskubernetes.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
				return cpustress.K8sAppDefinition(e, kubeProvider, "workload-stress")
			}),
			awskubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(helmValues)),
		)),
	}

	e2e.Run(t, &K8sSuite{}, options...)
}

func (s *K8sSuite) TestManualProcessCheck() {
	agent := getAgentPod(s.T(), s.Env().KubernetesCluster.Client())

	// The log level needs to be overridden as the pod has an ENV var set.
	// This is to so we get just json back from the check
	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.
		PodExec(agent.Namespace, agent.Name, "process-agent",
			[]string{"bash", "-c", "DD_LOG_LEVEL=OFF process-agent check process -w 5s --json"})
	assert.NoError(s.T(), err)
	assert.Empty(s.T(), stderr)

	assertManualProcessCheck(s.T(), stdout, false, "/usr/bin/stress-ng", "stress-ng")
}

func getAgentPod(t *testing.T, client kubeClient.Interface) corev1.Pod {
	res, err := client.CoreV1().Pods("datadog").
		List(context.Background(), v1.ListOptions{LabelSelector: "app=dda-linux-datadog"})
	require.NoError(t, err)
	require.NotEmpty(t, res.Items)

	return res.Items[0]
}

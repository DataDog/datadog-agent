// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"

	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/kindvm"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type kindSuite struct {
	k8sSuite
}

func TestKindSuite(t *testing.T) {
	suite.Run(t, &kindSuite{})
}

func (suite *kindSuite) SetupSuite() {
	ctx := context.Background()

	stackConfig := runner.ConfigMap{
		"ddagent:deploy":        auto.ConfigValue{Value: "true"},
		"ddagent:fakeintake":    auto.ConfigValue{Value: "true"},
		"ddtestworkload:deploy": auto.ConfigValue{Value: "true"},
	}

	_, stackOutput, err := infra.GetStackManager().GetStack(ctx, "kind-cluster", stackConfig, kindvm.Run, false)
	suite.Require().NoError(err)

	fakeintakeHost := stackOutput.Outputs["fakeintake-host"].Value.(string)
	suite.Fakeintake = fakeintake.NewClient(fmt.Sprintf("http://%s", fakeintakeHost))
	suite.KubeClusterName = stackOutput.Outputs["kube-cluster-name"].Value.(string)
	suite.AgentLinuxHelmInstallName = stackOutput.Outputs["agent-linux-helm-install-name"].Value.(string)
	suite.AgentWindowsHelmInstallName = "none"

	kubeconfig := stackOutput.Outputs["kubeconfig"].Value.(string)

	kubeconfigFile := path.Join(suite.T().TempDir(), "kubeconfig")
	suite.Require().NoError(os.WriteFile(kubeconfigFile, []byte(kubeconfig), 0600))

	suite.K8sConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigFile)
	suite.Require().NoError(err)

	suite.K8sClient = kubernetes.NewForConfigOrDie(suite.K8sConfig)

	suite.k8sSuite.SetupSuite()
}

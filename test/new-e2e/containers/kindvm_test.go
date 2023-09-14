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
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type kindSuite struct {
	k8sSuite
}

func TestKindSuite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	stackConfig := runner.ConfigMap{
		"ddagent:deploy":        auto.ConfigValue{Value: "true"},
		"ddagent:fakeintake":    auto.ConfigValue{Value: "true"},
		"ddtestworkload:deploy": auto.ConfigValue{Value: "true"},
	}

	_, stackOutput, err := infra.GetStackManager().GetStack(ctx, "kind-cluster", stackConfig, kindvm.Run, false)
	require.NoError(t, err)

	t.Cleanup(func() {
		infra.GetStackManager().DeleteStack(ctx, "kind-cluster")
	})

	fakeintakeHost := stackOutput.Outputs["fakeintake-host"].Value.(string)
	kubeconfig := stackOutput.Outputs["kubeconfig"].Value.(string)

	kubeconfigFile := path.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, os.WriteFile(kubeconfigFile, []byte(kubeconfig), 0600))

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigFile)
	require.NoError(t, err)

	suite.Run(t, &kindSuite{
		k8sSuite: k8sSuite{
			AgentLinuxHelmInstallName:   stackOutput.Outputs["agent-linux-helm-install-name"].Value.(string),
			AgentWindowsHelmInstallName: "none",
			Fakeintake:                  fakeintake.NewClient(fmt.Sprintf("http://%s", fakeintakeHost)),
			K8sConfig:                   config,
			K8sClient:                   kubernetes.NewForConfigOrDie(config),
		},
	})
}

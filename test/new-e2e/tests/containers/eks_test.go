// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/eks"

	"github.com/fatih/color"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type eksSuite struct {
	k8sSuite
}

func TestEKSSuite(t *testing.T) {
	ctx := context.Background()

	stackConfig := runner.ConfigMap{
		"ddagent:deploy":        auto.ConfigValue{Value: "true"},
		"ddagent:fakeintake":    auto.ConfigValue{Value: "true"},
		"ddtestworkload:deploy": auto.ConfigValue{Value: "true"},
	}

	_, stackOutput, err := infra.GetStackManager().GetStack(ctx, "eks-cluster", stackConfig, eks.Run, false)
	require.NoError(t, err)

	t.Cleanup(func() {
		infra.GetStackManager().DeleteStack(ctx, "eks-cluster")
	})

	fakeintakeHost := stackOutput.Outputs["fakeintake-host"].Value.(string)
	kubeconfig, err := json.Marshal(stackOutput.Outputs["kubeconfig"].Value.(map[string]interface{}))
	require.NoError(t, err)

	kubeconfigFile := path.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, os.WriteFile(kubeconfigFile, kubeconfig, 0600))

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigFile)
	require.NoError(t, err)

	startTime := time.Now()

	suite.Run(t, &eksSuite{
		k8sSuite: k8sSuite{
			baseSuite: baseSuite{
				Fakeintake: fakeintake.NewClient(fmt.Sprintf("http://%s", fakeintakeHost)),
			},
			AgentLinuxHelmInstallName:   stackOutput.Outputs["agent-linux-helm-install-name"].Value.(string),
			AgentWindowsHelmInstallName: stackOutput.Outputs["agent-windows-helm-install-name"].Value.(string),
			K8sConfig:                   config,
			K8sClient:                   kubernetes.NewForConfigOrDie(config),
		},
	})

	endTime := time.Now()

	color.NoColor = false
	c := color.New(color.Bold).SprintfFunc()
	t.Log(c("The data produced and asserted by these tests can be viewed on this dashboard:"))
	c = color.New(color.Bold, color.FgBlue).SprintfFunc()
	t.Log(c("https://dddev.datadoghq.com/dashboard/qcp-brm-ysc/e2e-tests-containers-k8s?refresh_mode=paused&tpl_var_kube_cluster_name%%5B0%%5D=%s&from_ts=%d&to_ts=%d&live=false",
		stackOutput.Outputs["kube-cluster-name"].Value.(string),
		startTime.UnixMilli(),
		endTime.UnixMilli(),
	))

}

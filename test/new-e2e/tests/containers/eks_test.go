// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/eks"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	awsekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/fatih/color"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	kubectlget "k8s.io/kubectl/pkg/cmd/get"
	kubectlutil "k8s.io/kubectl/pkg/cmd/util"
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

	_, stackOutput, err := infra.GetStackManager().GetStackNoDeleteOnFailure(ctx, "eks-cluster", stackConfig, eks.Run, false)
	if !assert.NoError(t, err) {
		stackName, err := infra.GetStackManager().GetPulumiStackName("eks-cluster")
		require.NoError(t, err)
		t.Log(dumpEKSClusterState(ctx, stackName))
		infra.GetStackManager().DeleteStack(ctx, "eks-cluster")
		t.FailNow()
	}

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

// dumpEKSClusterState re-implements in GO the following two lines of shell script:
//
//	aws eks update-kubeconfig --name $name
//	kubectl get nodes,all --all-namespaces -o wide
func dumpEKSClusterState(ctx context.Context, name string) string {
	var sb strings.Builder

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		fmt.Fprintf(&sb, "Failed to load AWS config: %v\n", err)
		return sb.String()
	}

	client := awseks.NewFromConfig(cfg)

	clusterDescription, err := client.DescribeCluster(ctx, &awseks.DescribeClusterInput{
		Name: &name,
	})
	if err != nil {
		fmt.Fprintf(&sb, "Failed to describe cluster %s: %v\n", name, err)
		return sb.String()
	}

	cluster := clusterDescription.Cluster
	if cluster.Status != awsekstypes.ClusterStatusActive {
		fmt.Fprintf(&sb, "EKS cluster %s is not in active state. Current status: %s\n", name, cluster.Status)
		return sb.String()
	}

	kubeconfig := clientcmdapi.NewConfig()
	kubeconfig.Clusters[name] = &clientcmdapi.Cluster{
		Server: *cluster.Endpoint,
	}
	if kubeconfig.Clusters[name].CertificateAuthorityData, err = base64.StdEncoding.DecodeString(*cluster.CertificateAuthority.Data); err != nil {
		fmt.Fprintf(&sb, "Failed to decode certificate authority: %v\n", err)
	}
	kubeconfig.AuthInfos[name] = &clientcmdapi.AuthInfo{
		Exec: &clientcmdapi.ExecConfig{
			APIVersion: "client.authentication.k8s.io/v1beta1",
			Command:    "aws",
			Args: []string{
				"--region",
				cfg.Region,
				"eks",
				"get-token",
				"--cluster-name",
				name,
				"--output",
				"json",
			},
		},
	}
	kubeconfig.Contexts[name] = &clientcmdapi.Context{
		Cluster:  name,
		AuthInfo: name,
	}
	kubeconfig.CurrentContext = name

	kubeconfigFile, err := os.CreateTemp("", "kubeconfig")
	if err != nil {
		fmt.Fprintf(&sb, "Failed to create kubeconfig temporary file: %v\n", err)
		return sb.String()
	}
	defer os.Remove(kubeconfigFile.Name())

	if err := clientcmd.WriteToFile(*kubeconfig, kubeconfigFile.Name()); err != nil {
		fmt.Fprintf(&sb, "Failed to write kubeconfig file: %v\n", err)
		return sb.String()
	}

	if err := kubeconfigFile.Close(); err != nil {
		fmt.Fprintf(&sb, "Failed to close kubeconfig file: %v\n", err)
	}

	configFlags := genericclioptions.NewConfigFlags(false)
	kubeconfigFileName := kubeconfigFile.Name()
	configFlags.KubeConfig = &kubeconfigFileName

	factory := kubectlutil.NewFactory(configFlags)

	streams := genericiooptions.IOStreams{
		Out:    &sb,
		ErrOut: &sb,
	}

	getCmd := kubectlget.NewCmdGet("", factory, streams)
	getCmd.SetOut(&sb)
	getCmd.SetErr(&sb)
	getCmd.SetContext(ctx)
	getCmd.SetArgs([]string{
		"nodes,all",
		"--all-namespaces",
		"-o",
		"wide",
	})
	if err := getCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(&sb, "Failed to execute Get command: %v\n", err)
		return sb.String()
	}

	return sb.String()
}

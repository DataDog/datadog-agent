// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	awsec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	awsekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	kubectlget "k8s.io/kubectl/pkg/cmd/get"
	kubectlutil "k8s.io/kubectl/pkg/cmd/util"
)

func dumpKubernetesClusterState(ctx context.Context, name string) string {

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return "WARNING: Failed to dump cluster state: failed to load AWS config"
	}

	dumpEKS, errEKS := tryDumpEKSClusterState(ctx, cfg, name)
	if errEKS == nil {
		return dumpEKS
	}

	dumpKind, errKind := tryDumpKindClusterState(ctx, cfg, name)
	if errKind == nil {
		return dumpKind
	}

	return fmt.Sprintf("WARNING: Failed to dump cluster state, tried EKS and Kind dump:\n EKS error: %v\n Kind error: %v", errEKS, errKind)
}
func tryDumpEKSClusterState(ctx context.Context, cfg aws.Config, name string) (ret string, err error) {
	var out strings.Builder
	defer func() { ret = out.String() }()

	client := awseks.NewFromConfig(cfg)

	clusterDescription, err := client.DescribeCluster(ctx, &awseks.DescribeClusterInput{
		Name: &name,
	})
	if err != nil {
		return "", fmt.Errorf("Failed to describe cluster %s: %v", name, err)
	}

	cluster := clusterDescription.Cluster
	if cluster.Status != awsekstypes.ClusterStatusActive {
		return "", fmt.Errorf("EKS cluster %s is not in active state. Current status: %s", name, cluster.Status)
	}

	kubeconfig := clientcmdapi.NewConfig()
	kubeconfig.Clusters[name] = &clientcmdapi.Cluster{
		Server: *cluster.Endpoint,
	}
	if kubeconfig.Clusters[name].CertificateAuthorityData, err = base64.StdEncoding.DecodeString(*cluster.CertificateAuthority.Data); err != nil {
		return "", fmt.Errorf("failed to decode certificate authority: %v", err)
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

	dumpK8sClusterState(ctx, kubeconfig, &out)

	return
}

func tryDumpKindClusterState(ctx context.Context, cfg aws.Config, name string) (ret string, err error) {
	var out strings.Builder
	defer func() { ret = out.String() }()

	ec2Client := awsec2.NewFromConfig(cfg)

	user, _ := user.Current()
	instancesDescription, err := ec2Client.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{
		Filters: []awsec2types.Filter{
			{
				Name:   pointer.Ptr("tag:managed-by"),
				Values: []string{"pulumi"},
			},
			{
				Name:   pointer.Ptr("tag:username"),
				Values: []string{user.Username},
			},
			{
				Name:   pointer.Ptr("tag:Name"),
				Values: []string{name + "-aws-kind"},
			},
		},
	})
	if err != nil {
		return ret, fmt.Errorf("Failed to describe instances: %v", err)
	}

	if instancesDescription == nil || (len(instancesDescription.Reservations) != 1 && len(instancesDescription.Reservations[0].Instances) != 1) {
		return ret, fmt.Errorf("Did not find exactly one instance for cluster %s", name)
	}

	instanceIP := instancesDescription.Reservations[0].Instances[0].PrivateIpAddress

	auth := []ssh.AuthMethod{}

	if sshAgentSocket, found := os.LookupEnv("SSH_AUTH_SOCK"); found {
		sshAgent, err := net.Dial("unix", sshAgentSocket)
		if err != nil {
			return "", fmt.Errorf("Failed to dial SSH agent: %v", err)
		}
		defer sshAgent.Close()

		auth = append(auth, ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers))
	}

	if sshKeyPath, found := os.LookupEnv("E2E_PRIVATE_KEY_PATH"); found {
		sshKey, err := os.ReadFile(sshKeyPath)
		if err != nil {
			return ret, fmt.Errorf("failed to read SSH key: %v", err)
		}

		signer, err := ssh.ParsePrivateKey(sshKey)
		if err != nil {
			return ret, fmt.Errorf("failed to parse SSH key: %v", err)
		}

		auth = append(auth, ssh.PublicKeys(signer))
	}

	sshClient, err := ssh.Dial("tcp", *instanceIP+":22", &ssh.ClientConfig{
		User:            "ubuntu",
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		return ret, fmt.Errorf("failed to dial SSH server %s: %v", *instanceIP, err)
	}
	defer sshClient.Close()

	sshSession, err := sshClient.NewSession()
	if err != nil {
		return ret, fmt.Errorf("failed to create SSH session: %v", err)
	}
	defer sshSession.Close()

	stdout, err := sshSession.StdoutPipe()
	if err != nil {
		return ret, fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	stderr, err := sshSession.StderrPipe()
	if err != nil {
		return ret, fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	err = sshSession.Start("kind get kubeconfig --name \"$(kind get clusters | head -n 1)\"")
	if err != nil {
		return ret, fmt.Errorf("failed to start remote command: %v", err)
	}

	var stdoutBuf bytes.Buffer

	var wg sync.WaitGroup
	wg.Add(2)
	errChannel := make(chan error, 2)

	go func() {
		if _, err := io.Copy(&stdoutBuf, stdout); err != nil {
			errChannel <- fmt.Errorf("failed to read stdout: %v", err)
		}
		wg.Done()
	}()

	go func() {
		if _, err := io.Copy(&out, stderr); err != nil {
			errChannel <- fmt.Errorf("failed to read stderr: %v", err)
		}
		wg.Done()
	}()

	err = sshSession.Wait()
	wg.Wait()
	close(errChannel)
	for err := range errChannel {
		if err != nil {
			return ret, err
		}
	}

	if err != nil {
		return ret, fmt.Errorf("remote command exited with error: %v", err)
	}

	kubeconfig, err := clientcmd.Load(stdoutBuf.Bytes())
	if err != nil {
		return ret, fmt.Errorf("Failed to parse kubeconfig: %v\n", err)
	}

	for _, cluster := range kubeconfig.Clusters {
		cluster.Server = strings.Replace(cluster.Server, "0.0.0.0", *instanceIP, 1)
		cluster.CertificateAuthorityData = nil
		cluster.InsecureSkipTLSVerify = true
	}

	err = dumpK8sClusterState(ctx, kubeconfig, &out)
	if err != nil {
		return ret, fmt.Errorf("failed to dump cluster state: %v", err)
	}

	return ret, nil
}

func dumpK8sClusterState(ctx context.Context, kubeconfig *clientcmdapi.Config, out *strings.Builder) error {
	kubeconfigFile, err := os.CreateTemp("", "kubeconfig")
	if err != nil {
		return fmt.Errorf("failed to create kubeconfig temporary file: %v", err)
	}
	defer os.Remove(kubeconfigFile.Name())

	if err := clientcmd.WriteToFile(*kubeconfig, kubeconfigFile.Name()); err != nil {
		return fmt.Errorf("failed to write kubeconfig file: %v", err)
	}

	if err := kubeconfigFile.Close(); err != nil {
		return fmt.Errorf("failed to close kubeconfig file: %v", err)
	}

	fmt.Fprintf(out, "\n")

	configFlags := genericclioptions.NewConfigFlags(false)
	kubeconfigFileName := kubeconfigFile.Name()
	configFlags.KubeConfig = &kubeconfigFileName

	factory := kubectlutil.NewFactory(configFlags)

	streams := genericiooptions.IOStreams{
		Out:    out,
		ErrOut: out,
	}

	getCmd := kubectlget.NewCmdGet("", factory, streams)
	getCmd.SetOut(out)
	getCmd.SetErr(out)
	getCmd.SetContext(ctx)
	getCmd.SetArgs([]string{
		"nodes,all",
		"--all-namespaces",
		"-o",
		"wide",
	})
	if err := getCmd.ExecuteContext(ctx); err != nil {
		return fmt.Errorf("failed to execute kubectl get: %v", err)
	}
	return nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package awskubernetes

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os/user"
	"strings"
	"sync"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	awsec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	awsekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/infra"
)

func DumpEKSClusterState(ctx context.Context, name string) (ret string, err error) {
	var out strings.Builder
	defer func() { ret = out.String() }()

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %v", err)
	}

	client := awseks.NewFromConfig(cfg)

	clusterDescription, err := client.DescribeCluster(ctx, &awseks.DescribeClusterInput{
		Name: &name,
	})
	if err != nil {
		return "", fmt.Errorf("failed to describe cluster %s: %v", name, err)
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

	err = infra.DumpK8sClusterState(ctx, kubeconfig, &out)
	if err != nil {
		return ret, fmt.Errorf("failed to dump cluster state: %v", err)
	}

	return
}

func DumpKindClusterState(ctx context.Context, name string) (ret string, err error) {
	var out strings.Builder
	defer func() { ret = out.String() }()

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %v", err)
	}
	ec2Client := awsec2.NewFromConfig(cfg)

	user, _ := user.Current()
	instanceName := name + "-aws-kind"
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
				Values: []string{instanceName},
			},
		},
	})
	if err != nil {
		return ret, fmt.Errorf("failed to describe instances: %v", err)
	}

	// instancesDescription.Reservations = []
	if instancesDescription == nil {
		return ret, fmt.Errorf("failed to describe instances, got nil result, err: %w", err)
	} else if len(instancesDescription.Reservations) == 0 {
		return ret, fmt.Errorf("did not find any reservations for cluster %s", instanceName)
	} else if len(instancesDescription.Reservations[0].Instances) != 1 {
		return ret, fmt.Errorf("did not find exactly one instance for cluster %s, found %d instead and %d reservations", instanceName, len(instancesDescription.Reservations[0].Instances), len(instancesDescription.Reservations))
	}

	instanceIP := instancesDescription.Reservations[0].Instances[0].PrivateIpAddress
	if instanceIP == nil {
		return ret, errors.New("failed to get private IP of instance")
	}

	sshClient, err := infra.SshConnectToInstance(*instanceIP, "22", "ubuntu")
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
		return ret, fmt.Errorf("failed to parse kubeconfig: %v", err)
	}

	for _, cluster := range kubeconfig.Clusters {
		cluster.Server = strings.Replace(cluster.Server, "0.0.0.0", *instanceIP, 1)
		cluster.CertificateAuthorityData = nil
		cluster.InsecureSkipTLSVerify = true
	}

	err = infra.DumpK8sClusterState(ctx, kubeconfig, &out)
	if err != nil {
		return ret, fmt.Errorf("failed to dump cluster state: %v", err)
	}

	return ret, nil
}

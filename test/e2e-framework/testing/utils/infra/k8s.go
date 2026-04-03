// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infra implements utilities to interact with a Pulumi infrastructure
package infra

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	kubectlget "k8s.io/kubectl/pkg/cmd/get"
	kubectlutil "k8s.io/kubectl/pkg/cmd/util"
)

func DumpK8sClusterState(ctx context.Context, kubeconfig *clientcmdapi.Config, out *strings.Builder) error {
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

	fmt.Fprintf(out, "\n---------- All resources ----------\n")

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

	// Get the logs of containers that have restarted
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigFile.Name())
	if err != nil {
		fmt.Fprintf(out, "Failed to build Kubernetes config: %v\n", err)
		return nil
	}
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(out, "Failed to create Kubernetes client: %v\n", err)
		return nil
	}

	pods, err := k8sClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Fprintf(out, "Failed to list pods: %v\n", err)
		return nil
	}

	for _, pod := range pods.Items {
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.RestartCount > 0 || !containerStatus.Ready {
				fmt.Fprintf(out, "\nLOGS FOR POD %s/%s CONTAINER %s:\n", pod.Namespace, pod.Name, containerStatus.Name)
				logs, err := k8sClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
					Container: containerStatus.Name,
					Previous:  true,
					TailLines: pointer.Ptr(int64(100)),
				}).Stream(ctx)
				if err != nil {
					fmt.Fprintf(out, "Failed to get logs: %v\n", err)
					continue
				}

				_, err = io.Copy(out, logs)
				logs.Close()
				if err != nil {
					fmt.Fprintf(out, "Failed to copy logs: %v\n", err)
					continue
				}
			}
		}
	}
	return nil
}

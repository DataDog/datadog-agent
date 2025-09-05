// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	kubeClient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// KubernetesClient is a wrapper around the k8s client library and provides convenience methods for interacting with a
// k8s cluster
type KubernetesClient struct {
	K8sConfig *rest.Config
	K8sClient kubeClient.Interface
}

// NewKubernetesClient creates a new KubernetesClient
func NewKubernetesClient(config *rest.Config) (*KubernetesClient, error) {
	// Create client
	k8sClient, err := kubeClient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &KubernetesClient{
		K8sConfig: config,
		K8sClient: k8sClient,
	}, nil
}

// PodExecOption is a function that can be used to modify the PodExecOptions
type PodExecOption func(*corev1.PodExecOptions)

// PodExec execs into a given namespace/pod and returns the output for the given command
func (k *KubernetesClient) PodExec(namespace, pod, container string, cmd []string, podOptions ...PodExecOption) (stdout, stderr string, err error) {
	req := k.K8sClient.CoreV1().RESTClient().Post().Resource("pods").Namespace(namespace).Name(pod).SubResource("exec")
	option := &corev1.PodExecOptions{
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
		Container: container,
		Command:   cmd,
	}

	for _, podOption := range podOptions {
		podOption(option)
	}

	req.VersionedParams(
		option,
		scheme.ParameterCodec,
	)

	exec, err := remotecommand.NewSPDYExecutor(k.K8sConfig, "POST", req.URL())
	if err != nil {
		return "", "", err
	}

	var stdoutSb, stderrSb strings.Builder
	err = exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdout: &stdoutSb,
		Stderr: &stderrSb,
	})
	if err != nil {
		return stdoutSb.String(), suppressGoCoverWarning(stderrSb.String()), err
	}

	return stdoutSb.String(), suppressGoCoverWarning(stderrSb.String()), nil
}

// DownloadFromPod downloads a folder from a pod to a local destination
func (k *KubernetesClient) DownloadFromPod(namespace, podName, container, srcPath, destPath string) error {
	reader, outStream := io.Pipe()
	options := &corev1.PodExecOptions{
		Container: container,
		Command:   []string{"tar", "cf", "-", srcPath},
		Stdout:    true,
		Stderr:    true,
	}

	req := k.K8sClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec")

	req.VersionedParams(options, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(k.K8sConfig, "POST", req.URL())
	if err != nil {
		return err
	}

	go func() {
		defer outStream.Close()
		err = exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
			Stdout: outStream,
			Stderr: os.Stderr,
			Tty:    false,
		})
	}()

	if err := os.MkdirAll(destPath, 0755); err != nil {
		return err
	}

	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		splittedSrcPath := strings.Split(srcPath, "/")
		target := filepath.Join(destPath, strings.TrimPrefix(header.Name, splittedSrcPath[len(splittedSrcPath)-1]))

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			dir := filepath.Dir(target)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}

			file, err := os.Create(target)
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(file, tarReader); err != nil {
				return err
			}
		}
	}

	return nil
}

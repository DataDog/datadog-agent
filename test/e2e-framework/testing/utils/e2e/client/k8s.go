// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	// Tar from the parent directory to avoid embedding parent folders (e.g., /tmp)

	parentDir := filepath.Dir(srcPath)
	baseName := filepath.Base(srcPath)
	options := &corev1.PodExecOptions{
		Container: container,
		// Use options first to satisfy both GNU tar and BusyBox tar
		Command: []string{"tar", "cf", "-", "-C", parentDir, baseName},
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
		TTY:     false,
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

	// Capture stream errors from the goroutine
	streamErrCh := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	go func() {
		var stderrSb strings.Builder

		streamErr := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdout: outStream,
			Stderr: &stderrSb,
			Tty:    false,
		})
		if streamErr != nil && !errors.Is(streamErr, context.Canceled) {
			_ = outStream.CloseWithError(fmt.Errorf("stream error: %w, %s", streamErr, stderrSb.String()))
			streamErrCh <- streamErr
			return
		}
		_ = outStream.Close()
		streamErrCh <- nil
	}()

	if err := os.MkdirAll(destPath, 0755); err != nil {
		return err
	}

	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			// Reading the tar file is complete let's cancel the execution stream, as it is not stopped and will hang otherwise.
			cancel()
			break
		}
		if err != nil {
			return err
		}
		// Strip the top-level folder (baseName) so contents land directly in destPath
		entryName := strings.TrimPrefix(header.Name, "./")
		if entryName == baseName || entryName == baseName+"/" {
			// Skip the root directory entry
			continue
		}
		if after, ok := strings.CutPrefix(entryName, baseName+"/"); ok {
			entryName = after
		}
		target := filepath.Join(destPath, entryName)

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
			if _, err := io.Copy(file, tarReader); err != nil {
				_ = file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		}
	}
	// Ensure the exec stream completed successfully
	streamErr := <-streamErrCh

	return streamErr
}

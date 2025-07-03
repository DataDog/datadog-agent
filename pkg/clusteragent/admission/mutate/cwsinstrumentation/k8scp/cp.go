// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package k8scp implements the necessary methods to copy a local file to a remote
// container
package k8scp

import (
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/cwsinstrumentation/k8sexec"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

var (
	// CWSRemoteCopyCommand is the command used to copy cws-instrumentation, arguments are split on purpose
	// to try to differentiate this kubectl cp from others
	CWSRemoteCopyCommand = []string{"tar", "-x", "-m", "-f", "-"}
)

// Copy perform remote copy operations
type Copy struct {
	k8sexec.Exec
}

// NewCopy creates a Copy instance
func NewCopy(apiClient *apiserver.APIClient) *Copy {
	return &Copy{
		Exec: k8sexec.NewExec(apiClient),
	}
}

func (o *Copy) prepareCommand(destFile string) []string {
	// arguments are split on purpose to try to differentiate this kubectl cp from others
	cmdArr := make([]string, len(CWSRemoteCopyCommand))
	copy(cmdArr, CWSRemoteCopyCommand)
	destFileDir := path.Dir(destFile)
	if len(destFileDir) > 0 {
		cmdArr = append(cmdArr, "-C", destFileDir)
	}
	return cmdArr
}

// CopyToPod copies the provided local file to the provided container while observing the execution time
func (o *Copy) CopyToPod(localFile string, remoteFile string, pod *corev1.Pod, container string, mode string, webhookName string, timeout time.Duration) error {
	start := time.Now()
	err := o.copyToPod(localFile, remoteFile, pod, container, mode, webhookName, timeout)
	metrics.CWSResponseDuration.Observe(time.Since(start).Seconds(), mode, webhookName, "copy_to_pod", strconv.FormatBool(err == nil), "")
	return err
}

// copyToPod copies the provided local file to the provided container
func (o *Copy) copyToPod(localFile string, remoteFile string, pod *corev1.Pod, container string, mode string, webhookName string, timeout time.Duration) error {
	o.Container = container
	localFile = path.Clean(localFile)
	remoteFile = path.Clean(remoteFile)

	// sanity check
	if _, err := os.Stat(localFile); err != nil {
		return fmt.Errorf("%s doesn't exist in local filesystem", localFile)
	}

	reader, writer := io.Pipe()
	defer func() {
		_ = reader.Close()
		_ = writer.Close()
	}()
	tarErrChan := make(chan error, 1)
	go func(src, dest string, writer io.WriteCloser) {
		tarStart := time.Now()
		defer func() {
			metrics.CWSResponseDuration.Observe(time.Since(tarStart).Seconds(), mode, webhookName, "copy_to_pod_tar", "true", "")
		}()
		if err := makeTar(src, dest, writer); err != nil {
			_ = writer.Close()
			tarErrChan <- fmt.Errorf("failed to tar local file: %v", err)
			return
		}
		if err := writer.Close(); err != nil {
			tarErrChan <- fmt.Errorf("failed to close the pipe writer: %v", err)
			return
		}
		tarErrChan <- nil
	}(localFile, remoteFile, writer)

	streamOptions := k8sexec.StreamOptions{
		IOStreams: genericiooptions.IOStreams{
			In:     reader,
			Out:    o.Out,
			ErrOut: o.ErrOut,
		},
		Stdin: true,
	}

	start := time.Now()
	if err := o.Execute(pod, o.prepareCommand(remoteFile), streamOptions, mode, webhookName, timeout); err != nil {
		metrics.CWSResponseDuration.Observe(time.Since(start).Seconds(), mode, webhookName, "copy_to_pod_cmd_execute", "false", "")
		return fmt.Errorf("command execute error (in %s): %v", time.Since(start), err)
	}
	metrics.CWSResponseDuration.Observe(time.Since(start).Seconds(), mode, webhookName, "copy_to_pod_cmd_execute", "true", "")

	// close pipe, wait for tar chan to finish and check tar error
	tarErr := <-tarErrChan
	if tarErr != nil && len(tarErr.Error()) > 0 {
		return tarErr
	}

	// check stdout and stderr from tar
	outData := o.ErrOut.String() + o.Out.String()
	if len(outData) > 0 {
		return fmt.Errorf("unexpected output: '%s' (%d)", outData, len(outData))
	}
	return nil
}

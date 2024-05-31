// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package k8scp implements the necessary methods to copy a local file to a remote
// container
package k8scp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// CWSRemoteCopyCommand is the command used to copy cws-instrumentation, arguments are split on purpose
	// to try to differentiate this kubectl cp from others
	CWSRemoteCopyCommand = []string{"tar", "-x", "-m", "-f", "-"}
)

// Copy perform remote copy operations
type Copy struct {
	Container string
	Namespace string

	apiClient *apiserver.APIClient
	streams   genericiooptions.IOStreams
	in        *bytes.Buffer
	out       *bytes.Buffer
	errOut    *bytes.Buffer
}

// NewCopy creates a Command instance
func NewCopy(apiClient *apiserver.APIClient) *Copy {
	ioStreams, in, out, errOut := genericiooptions.NewTestIOStreams()
	return &Copy{
		streams:   ioStreams,
		in:        in,
		out:       out,
		errOut:    errOut,
		apiClient: apiClient,
	}
}

// CopyToPod copies the provided local file to the provided container
func (o *Copy) CopyToPod(localFile string, remoteFile string, pod *corev1.Pod, container string) error {
	o.Container = container

	// sanity check
	if _, err := os.Stat(localFile); err != nil {
		return fmt.Errorf("%s doesn't exist in local filesystem", localFile)
	}

	reader, writer := io.Pipe()
	srcFile := newLocalPath(localFile)
	destFile := newRemotePath(remoteFile)

	go func(src localPath, dest remotePath, writer io.WriteCloser) {
		defer writer.Close()
		if err := makeTar(src, dest, writer); err != nil {
			log.Debugf("failed to tar local file: %v", err)
		}
	}(srcFile, destFile, writer)

	// arguments are split on purpose to try to differentiate this kubectl cp from others
	cmdArr := make([]string, len(CWSRemoteCopyCommand))
	copy(cmdArr, CWSRemoteCopyCommand)
	destFileDir := destFile.Dir().String()
	if len(destFileDir) > 0 {
		cmdArr = append(cmdArr, "-C", destFileDir)
	}

	streamOptions := StreamOptions{
		IOStreams: genericiooptions.IOStreams{
			In:     reader,
			Out:    o.out,
			ErrOut: o.errOut,
		},
		Stdin: true,
	}
	return o.execute(pod, cmdArr, streamOptions)
}

func (o *Copy) execute(pod *corev1.Pod, command []string, streamOptions StreamOptions) error {
	// ensure we can recover the terminal while attached
	t := streamOptions.SetupTTY()

	var sizeQueue remotecommand.TerminalSizeQueue
	if t.Raw {
		// this call spawns a goroutine to monitor/update the terminal size
		sizeQueue = t.MonitorSize(t.GetSize())

		// unset p.Err if it was previously set because both stdout and stderr go over p.Out when tty is
		// true
		streamOptions.ErrOut = nil
	}

	fn := func() error {
		restClient, err := o.apiClient.RESTClient(
			"/api",
			&schema.GroupVersion{Version: "v1"},
			serializer.WithoutConversionCodecFactory{CodecFactory: scheme.Codecs},
		)
		if err != nil {
			return err
		}

		req := restClient.Post().
			Resource("pods").
			Name(pod.Name).
			Namespace(pod.Namespace).
			SubResource("exec")
		req.VersionedParams(&corev1.PodExecOptions{
			Container: o.Container,
			Command:   command,
			Stdin:     streamOptions.Stdin,
			Stdout:    streamOptions.Out != nil,
			Stderr:    streamOptions.ErrOut != nil,
			TTY:       t.Raw,
		}, scheme.ParameterCodec)

		exec, err := o.apiClient.NewSPDYExecutor(
			"/api",
			&schema.GroupVersion{Version: "v1"},
			serializer.WithoutConversionCodecFactory{CodecFactory: scheme.Codecs},
			"POST",
			req.URL(),
		)
		if err != nil {
			return err
		}

		return exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
			Stdin:             streamOptions.In,
			Stdout:            streamOptions.Out,
			Stderr:            streamOptions.ErrOut,
			Tty:               t.Raw,
			TerminalSizeQueue: sizeQueue,
		})
	}

	return t.Safe(fn)
}

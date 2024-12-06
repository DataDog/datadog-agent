// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package k8sexec implements the necessary methods to run commands remotely
package k8sexec

import (
	"bytes"
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// StreamOptions contains option for the remote stream
type StreamOptions struct {
	Stdin bool
	genericiooptions.IOStreams
}

// Exec performs remote exec operations
type Exec struct {
	Container string
	Namespace string

	APIClient *apiserver.APIClient
	In        *bytes.Buffer
	Out       *bytes.Buffer
	ErrOut    *bytes.Buffer
}

// NewExec creates a Exec instance
func NewExec(apiClient *apiserver.APIClient) Exec {
	return Exec{
		In:        &bytes.Buffer{},
		Out:       &bytes.Buffer{},
		ErrOut:    &bytes.Buffer{},
		APIClient: apiClient,
	}
}

// Execute runs the exec command
func (e Exec) Execute(pod *corev1.Pod, command []string, streamOptions StreamOptions) error {
	restClient, err := e.APIClient.RESTClient(
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
		Container: e.Container,
		Command:   command,
		Stdin:     streamOptions.Stdin,
		Stdout:    streamOptions.Out != nil,
		Stderr:    streamOptions.ErrOut != nil,
	}, scheme.ParameterCodec)

	exec, err := e.APIClient.NewSPDYExecutor(
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
		Stdin:  streamOptions.In,
		Stdout: streamOptions.Out,
		Stderr: streamOptions.ErrOut,
	})
}

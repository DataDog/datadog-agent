// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_core

import (
	"context"
	"errors"
	"fmt"
	"io"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const maxPodLogsBytes int64 = 10 * 1024 * 1024

// GetPodLogsHandler retrieves the finite log output for one Pod container.
type GetPodLogsHandler struct{}

// NewGetPodLogsHandler creates a Pod logs handler.
func NewGetPodLogsHandler() *GetPodLogsHandler {
	return &GetPodLogsHandler{}
}

// GetPodLogsInputs selects the Pod log stream and bounds the returned history.
// Follow and insecureSkipTLSVerifyBackend are intentionally not exposed because
// private actions must terminate and retain the Kubernetes client's TLS checks.
type GetPodLogsInputs struct {
	Name         string       `json:"name"`
	Namespace    string       `json:"namespace"`
	Container    string       `json:"container,omitempty"`
	Previous     bool         `json:"previous,omitempty"`
	SinceSeconds *int64       `json:"sinceSeconds,omitempty"`
	SinceTime    *metav1.Time `json:"sinceTime,omitempty"`
	Timestamps   bool         `json:"timestamps,omitempty"`
	TailLines    *int64       `json:"tailLines,omitempty"`
	LimitBytes   *int64       `json:"limitBytes,omitempty"`
}

// GetPodLogsOutputs contains the selected Pod logs.
type GetPodLogsOutputs struct {
	Logs string `json:"logs"`
}

// Run retrieves logs through the Kubernetes Pod log subresource.
func (h *GetPodLogsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[GetPodLogsInputs](task)
	if err != nil {
		return nil, err
	}
	if err := inputs.validate(); err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	request := client.CoreV1().Pods(inputs.Namespace).GetLogs(inputs.Name, inputs.options())
	logs, err := readPodLogs(ctx, request.Stream)
	if err != nil {
		return nil, err
	}
	return &GetPodLogsOutputs{Logs: logs}, nil
}

func (inputs GetPodLogsInputs) validate() error {
	if inputs.Name == "" {
		return errors.New("pod name is required")
	}
	if inputs.Namespace == "" {
		return errors.New("pod namespace is required")
	}
	if inputs.SinceSeconds != nil && inputs.SinceTime != nil {
		return errors.New("only one of sinceSeconds and sinceTime may be specified")
	}
	if inputs.SinceSeconds != nil && *inputs.SinceSeconds <= 0 {
		return errors.New("sinceSeconds must be positive")
	}
	if inputs.TailLines != nil && *inputs.TailLines < 0 {
		return errors.New("tailLines must not be negative")
	}
	if inputs.LimitBytes != nil {
		if *inputs.LimitBytes <= 0 {
			return errors.New("limitBytes must be positive")
		}
		if *inputs.LimitBytes > maxPodLogsBytes {
			return fmt.Errorf("limitBytes must not exceed %d bytes", maxPodLogsBytes)
		}
	}
	return nil
}

func (inputs GetPodLogsInputs) options() *corev1.PodLogOptions {
	limitBytes := inputs.LimitBytes
	if limitBytes == nil {
		defaultLimit := maxPodLogsBytes
		limitBytes = &defaultLimit
	}
	return &corev1.PodLogOptions{
		Container:    inputs.Container,
		Previous:     inputs.Previous,
		SinceSeconds: inputs.SinceSeconds,
		SinceTime:    inputs.SinceTime,
		Timestamps:   inputs.Timestamps,
		TailLines:    inputs.TailLines,
		LimitBytes:   limitBytes,
	}
}

func readPodLogs(ctx context.Context, stream func(context.Context) (io.ReadCloser, error)) (string, error) {
	reader, err := stream(ctx)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	contents, err := io.ReadAll(io.LimitReader(reader, maxPodLogsBytes+1))
	if err != nil {
		return "", fmt.Errorf("could not read pod logs: %w", err)
	}
	if int64(len(contents)) > maxPodLogsBytes {
		return "", fmt.Errorf("pod logs exceed the %d byte output limit", maxPodLogsBytes)
	}
	return string(contents), nil
}

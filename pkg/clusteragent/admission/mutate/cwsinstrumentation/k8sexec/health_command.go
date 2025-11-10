// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package k8sexec

import (
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/DataDog/datadog-agent/cmd/cws-instrumentation/subcommands/healthcmd"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

var (
	// CWSHealthCommand is the command used to check if cws-instrumentation was properly copied to the target container
	CWSHealthCommand = "health"
)

// HealthCommand performs remote exec operations
type HealthCommand struct {
	Exec
}

// NewHealthCommand creates an Exec instance
func NewHealthCommand(apiClient *apiserver.APIClient) *HealthCommand {
	return &HealthCommand{
		Exec: NewExec(apiClient),
	}
}

func (hc *HealthCommand) prepareCommand(destFile string) []string {
	return []string{
		destFile,
		CWSHealthCommand,
	}
}

// Run runs the cws-instrumentation health command
func (hc *HealthCommand) Run(remoteFile string, pod *corev1.Pod, container string, mode string, webhookName string, timeout time.Duration) error {
	start := time.Now()
	err := hc.run(remoteFile, pod, container, mode, webhookName, timeout)
	metrics.CWSResponseDuration.Observe(time.Since(start).Seconds(), mode, webhookName, "health_command", strconv.FormatBool(err == nil), "")
	return err
}

func (hc *HealthCommand) run(remoteFile string, pod *corev1.Pod, container string, mode string, webhookName string, timeout time.Duration) error {
	hc.Container = container

	streamOptions := StreamOptions{
		IOStreams: genericiooptions.IOStreams{
			Out:    hc.Out,
			ErrOut: hc.ErrOut,
		},
		Stdin: false,
	}

	if err := hc.Execute(pod, hc.prepareCommand(remoteFile), streamOptions, mode, webhookName, timeout); err != nil {
		return err
	}

	// check stdout and stderr from tar
	outData := hc.ErrOut.String() + hc.Out.String()
	if outData != healthcmd.HealthCommandOutput {
		return fmt.Errorf("unexpected output (\"%s\"): cws-instrumentation might not have been copied properly", outData)
	}
	return nil
}

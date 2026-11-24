// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package environments

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubeoutput "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
)

func TestDescribeRestart(t *testing.T) {
	t.Run("unknown last termination", func(t *testing.T) {
		assert.Equal(t, "restarted 1 time(s)", describeRestart(corev1.ContainerStatus{RestartCount: 1}))
	})

	t.Run("OOM kill", func(t *testing.T) {
		status := restartedStatus("agent", 1, &corev1.ContainerStateTerminated{
			Reason:     "OOMKilled",
			ExitCode:   137,
			FinishedAt: metav1.NewTime(time.Date(2026, 11, 24, 16, 42, 54, 0, time.UTC)),
		})
		assert.Equal(t, "restarted 1 time(s) (last termination: reason OOMKilled, exit code 137, at 2026-11-24T16:42:54Z)", describeRestart(status))
	})

	t.Run("signal and multi-line message", func(t *testing.T) {
		status := restartedStatus("agent", 2, &corev1.ContainerStateTerminated{
			ExitCode: 2,
			Signal:   6,
			Message:  "panic: boom\n\ngoroutine 1\n",
		})
		assert.Equal(t, "restarted 2 time(s) (last termination: reason unknown, exit code 2, signal 6, message: panic: boom goroutine 1)", describeRestart(status))
	})
}

func TestCheckAgentPodsReadyReportsLastTermination(t *testing.T) {
	pod := readyPod("agent-1", "agent", 1)
	pod.Status.ContainerStatuses[0].LastTerminationState.Terminated = &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 2}
	env := newReadinessTestEnvironment(linuxNode("node-1"), pod)

	err := checkAgentPodsReady(context.Background(), env.KubernetesCluster, []podReadinessTarget{{
		name:          "linux node agent",
		namespace:     agentNamespace,
		labelSelector: "app=agent",
		nodeSelector:  "kubernetes.io/os=linux",
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "container agent in pod agent-1 restarted 1 time(s) (last termination: reason Error, exit code 2)")
}

func TestDiagnoseRestartedContainers(t *testing.T) {
	pod := readyPod("agent-1", "agent", 0)
	pod.Status.InitContainerStatuses = []corev1.ContainerStatus{restartedStatus("init-config", 1, nil)}
	pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses,
		restartedStatus("trace-agent", 3, &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}))
	env := newReadinessTestEnvironment(pod)
	outputDir := t.TempDir()

	output := env.diagnoseRestartedContainers(context.Background(), &previousLogsBudget{remaining: previousLogsTotalTimeout}, *pod, outputDir)

	// The fake clientset answers every log request with "fake logs".
	initLogs := filepath.Join(outputDir, "agent-1", "init-config-previous.log")
	traceLogs := filepath.Join(outputDir, "agent-1", "trace-agent-previous.log")
	assert.Equal(t, []string{
		"Container init-config restarted 1 time(s)",
		"Logs of the previous instance of container init-config: " + initLogs,
		"    fake logs",
		"Container trace-agent restarted 3 time(s) (last termination: reason OOMKilled, exit code 137)",
		"Logs of the previous instance of container trace-agent: " + traceLogs,
		"    fake logs",
	}, output)
	for _, path := range []string{initLogs, traceLogs} {
		logs, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "fake logs", string(logs))
	}
}

func TestDiagnoseClusterChecksRunnerWithoutNodeAgents(t *testing.T) {
	pod := readyPod("ccr-1", "ccr", 0)
	pod.Namespace = "custom"
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{
		restartedStatus("agent", 1, &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 2}),
	}
	env := newReadinessTestEnvironment(pod)
	env.Agent.LinuxClusterChecks = kubeoutput.KubernetesObjRefOutput{Namespace: "custom", LabelSelectors: map[string]string{"app": "ccr"}}
	outputDir := t.TempDir()

	output, err := env.Diagnose(outputDir)

	require.NoError(t, err)
	// The missing Linux Node Agent pods don't stop the diagnose of the other workloads.
	assert.Contains(t, output, "No linux pods found")
	assert.Contains(t, output, "==== Cluster Checks Runner pods ====\nPod ccr-1:\n\nContainer agent restarted 1 time(s) (last termination: reason Error, exit code 2)")
	assert.FileExists(t, filepath.Join(outputDir, "ccr-1", "agent-previous.log"))
}

func TestPreviousLogsBudget(t *testing.T) {
	t.Run("pod timeout", func(t *testing.T) {
		budget := &previousLogsBudget{remaining: previousLogsTotalTimeout}
		ctx, done := budget.withTimeout(context.Background())
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		assert.WithinDuration(t, time.Now().Add(previousLogsPodTimeout), deadline, time.Second)
		done()
		assert.Less(t, budget.remaining, previousLogsTotalTimeout)
	})

	t.Run("exhausted budget", func(t *testing.T) {
		budget := &previousLogsBudget{}
		ctx, done := budget.withTimeout(context.Background())
		defer done()
		assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	})
}

func TestCrashExcerpt(t *testing.T) {
	var lines []string
	for i := range 50 {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}

	t.Run("empty logs", func(t *testing.T) {
		assert.Empty(t, crashExcerpt(" \n"))
	})

	t.Run("no crash report", func(t *testing.T) {
		excerpt := crashExcerpt(strings.Join(lines, "\n") + "\n")
		require.Len(t, excerpt, crashExcerptLines)
		assert.Equal(t, "    line 30", excerpt[0])
		assert.Equal(t, "    line 49", excerpt[len(excerpt)-1])
	})

	t.Run("panic", func(t *testing.T) {
		withPanic := slices.Concat(lines[:10], []string{"panic: runtime error: invalid memory address", "", "goroutine 42 [running]:"}, lines[10:])
		excerpt := crashExcerpt(strings.Join(withPanic, "\n"))
		require.Len(t, excerpt, crashExcerptLines)
		assert.Equal(t, "    panic: runtime error: invalid memory address", excerpt[0])
		assert.Equal(t, "    goroutine 42 [running]:", excerpt[2])
	})

	t.Run("long line", func(t *testing.T) {
		excerpt := crashExcerpt(strings.Repeat("x", 2*crashExcerptLineLength))
		assert.Equal(t, []string{"    " + strings.Repeat("x", crashExcerptLineLength) + "…"}, excerpt)
	})

	t.Run("fatal signal near the end", func(t *testing.T) {
		excerpt := crashExcerpt(strings.Join(slices.Concat(lines, []string{"SIGSEGV: segmentation violation", "PC=0x0"}), "\n"))
		assert.Equal(t, []string{"    SIGSEGV: segmentation violation", "    PC=0x0"}, excerpt)
	})
}

func restartedStatus(name string, restartCount int32, terminated *corev1.ContainerStateTerminated) corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:                 name,
		Ready:                true,
		RestartCount:         restartCount,
		LastTerminationState: corev1.ContainerState{Terminated: terminated},
	}
}

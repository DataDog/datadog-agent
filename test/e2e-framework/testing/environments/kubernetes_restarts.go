// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package environments

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// previousLogsPodTimeout and previousLogsTotalTimeout bound the time Diagnose spends getting the
// logs of the previous instances of the containers that restarted, in a pod and in all of them, so
// that kubelets that don't answer can't hold the other diagnostics and the teardown up.
const (
	previousLogsPodTimeout   = 30 * time.Second
	previousLogsTotalTimeout = 2 * time.Minute
)

// previousLogsBudget is the time Diagnose has left to get logs of previous container instances. It
// only decreases while getting them, not while creating flares in between.
type previousLogsBudget struct {
	remaining time.Duration
}

// withTimeout returns a context bounded by previousLogsPodTimeout and the remaining budget, and a
// function to call once done, which takes the elapsed time off the budget.
func (b *previousLogsBudget) withTimeout(ctx context.Context) (context.Context, func()) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, min(previousLogsPodTimeout, b.remaining))
	return ctx, func() {
		cancel()
		b.remaining = max(b.remaining-time.Since(start), 0)
	}
}

// crashExcerptLines is the number of lines of the logs of a previous container instance that
// the diagnose prints, from the start of its crash report or else from the end of the logs.
const crashExcerptLines = 20

// crashExcerptLineLength is the length beyond which the lines of the excerpt are truncated.
const crashExcerptLineLength = 500

// crashStart matches the first line of the crash report of a Go program.
var crashStart = regexp.MustCompile(`^(panic: |fatal error: |SIG[A-Z]+: |unexpected signal )`)

// describeRestart describes the restarts of a container and, when known, its last termination, e.g.
// "restarted 1 time(s) (last termination: reason OOMKilled, exit code 137, at 2026-11-24T16:42:54Z)".
func describeRestart(status corev1.ContainerStatus) string {
	description := fmt.Sprintf("restarted %d time(s)", status.RestartCount)
	terminated := status.LastTerminationState.Terminated
	if terminated == nil {
		return description
	}
	description += fmt.Sprintf(" (last termination: reason %s, exit code %d", cmp.Or(terminated.Reason, "unknown"), terminated.ExitCode)
	if terminated.Signal != 0 {
		description += fmt.Sprintf(", signal %d", terminated.Signal)
	}
	if !terminated.FinishedAt.IsZero() {
		description += ", at " + terminated.FinishedAt.UTC().Format(time.RFC3339)
	}
	if message := strings.TrimSpace(terminated.Message); message != "" {
		description += ", message: " + strings.Join(strings.Fields(message), " ")
	}
	return description + ")"
}

// diagnoseRestartedContainers reports the containers of pod that restarted, with their last
// termination, and saves the logs of their previous instance under outputDir/<pod>/. A flare only
// holds the logs of the running instance, and the Agent can't ship the last logs of an instance
// that crashed, e.g. the stack trace of a panic, which goes to stderr.
func (e *Kubernetes) diagnoseRestartedContainers(ctx context.Context, budget *previousLogsBudget, pod corev1.Pod, outputDir string) []string {
	ctx, done := budget.withTimeout(ctx)
	defer done()

	var output []string
	for _, status := range slices.Concat(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses) {
		if status.RestartCount == 0 {
			continue
		}
		output = append(output, fmt.Sprintf("Container %s %s", status.Name, describeRestart(status)))

		logs, err := e.KubernetesCluster.Client().CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
			Container: status.Name,
			Previous:  true,
		}).DoRaw(ctx)
		if err != nil {
			output = append(output, fmt.Sprintf("Failed to get the logs of the previous instance of container %s: %v", status.Name, err))
			continue
		}
		logsPath := filepath.Join(outputDir, pod.Name, status.Name+"-previous.log")
		err = os.MkdirAll(filepath.Dir(logsPath), 0o755)
		if err == nil {
			err = os.WriteFile(logsPath, logs, 0o644)
		}
		if err != nil {
			output = append(output, fmt.Sprintf("Failed to save the logs of the previous instance of container %s: %v", status.Name, err))
		} else {
			output = append(output, fmt.Sprintf("Logs of the previous instance of container %s: %s", status.Name, logsPath))
		}
		output = append(output, crashExcerpt(string(logs))...)
	}
	return output
}

// crashExcerpt returns, indented, the start of the first Go crash report in logs or, if there is
// none, the last lines of logs, or nil for empty logs.
func crashExcerpt(logs string) []string {
	if strings.TrimSpace(logs) == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(logs, "\n"), "\n")
	start := max(len(lines)-crashExcerptLines, 0)
	for i, line := range lines {
		if crashStart.MatchString(line) {
			start = i
			break
		}
	}
	excerpt := lines[start:min(start+crashExcerptLines, len(lines))]
	indented := make([]string, 0, len(excerpt))
	for _, line := range excerpt {
		// Windows containers can end their lines with "\r\n".
		line = strings.TrimSuffix(line, "\r")
		if len(line) > crashExcerptLineLength {
			line = line[:crashExcerptLineLength] + "…"
		}
		indented = append(indented, "    "+line)
	}
	return indented
}

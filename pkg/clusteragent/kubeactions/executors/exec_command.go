// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package executors

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"
)

const (
	// rshellInjectedAnnotation is stamped on pods mutated by the rshell injection
	// webhook (pkg/clusteragent/admission/mutate/rshell). Its presence is the
	// fail-fast signal that the rshell binary is available inside the pod.
	rshellInjectedAnnotation = "rshell.datadoghq.com/injected"
	// rshellBinaryPathAnnotation records where the injected binary lives in the pod.
	rshellBinaryPathAnnotation = "rshell.datadoghq.com/binary-path"
	// defaultRshellBinaryPath is the fallback exec path when the annotation is absent.
	defaultRshellBinaryPath = "/datadog-rshell/rshell"

	// maxExecOutputBytes bounds each captured stream well under the Event Platform
	// message limit. Output beyond this is truncated and flagged.
	maxExecOutputBytes = 1 * 1024 * 1024 // 1 MiB

	// execStreamGrace is added to the rshell --timeout so rshell can self-terminate
	// and flush its result before the stream context is cancelled out from under it.
	execStreamGrace = 5 * time.Second
)

// ExecCommandExecutor runs a restricted shell program inside a container via the
// injected rshell binary and captures bounded stdout/stderr plus the exit code.
type ExecCommandExecutor struct {
	clientset  kubernetes.Interface
	restConfig *rest.Config
}

var _ Executor = (*ExecCommandExecutor)(nil)

// NewExecCommandExecutor creates a new ExecCommandExecutor.
func NewExecCommandExecutor(clientset kubernetes.Interface, restConfig *rest.Config) *ExecCommandExecutor {
	return &ExecCommandExecutor{
		clientset:  clientset,
		restConfig: restConfig,
	}
}

// Execute runs the rshell program in the target container.
func (e *ExecCommandExecutor) Execute(ctx context.Context, action *kubeactions.KubeAction) ExecutionResult {
	resource := action.Resource
	namespace := resource.Namespace
	name := resource.Name
	resourceID := resource.ResourceId

	params := action.GetExecCommand()
	if params == nil {
		return ExecutionResult{Status: StatusFailed, Message: "exec_command parameters are required"}
	}

	// Get the pod first to verify UID matches resource_id (guards against the pod
	// being replaced between action creation and execution).
	pod, err := e.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ExecutionResult{Status: StatusFailed, Message: fmt.Sprintf("failed to get pod: %v", err)}
	}
	if string(pod.UID) != resourceID {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("pod UID mismatch: expected %s, got %s - pod may have been replaced since action was created", resourceID, pod.UID),
		}
	}

	// Fail fast if the rshell binary was never injected into this pod.
	if _, ok := pod.Annotations[rshellInjectedAnnotation]; !ok {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("pod %s/%s has no rshell binary injected (missing %q annotation); enable rshell injection on this workload", namespace, name, rshellInjectedAnnotation),
		}
	}
	binaryPath := pod.Annotations[rshellBinaryPathAnnotation]
	if binaryPath == "" {
		binaryPath = defaultRshellBinaryPath
	}

	container := params.GetContainer()
	if !podHasContainer(pod, container) {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("container %q not found in pod %s/%s", container, namespace, name),
		}
	}

	// Resolve the effective (narrowed) policy and build the rshell command line.
	policy := resolveExecPolicy(params)
	command := buildRshellCommand(binaryPath, params.GetScript(), policy)

	ctx, cancel := context.WithTimeout(ctx, policy.timeout+execStreamGrace)
	defer cancel()

	req := e.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(e.restConfig, "POST", req.URL())
	if err != nil {
		return ExecutionResult{Status: StatusFailed, Message: fmt.Sprintf("failed to initialize exec stream: %v", err)}
	}

	stdout := &cappedBuffer{limit: maxExecOutputBytes}
	stderr := &cappedBuffer{limit: maxExecOutputBytes}

	var exitCode int32
	streamErr := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})

	message := fmt.Sprintf("exec_command completed in %s/%s container %q", namespace, name, container)
	if streamErr != nil {
		// A non-zero rshell exit is a completed run, not an infrastructure failure:
		// the exit code is surfaced as the structured result. Any other stream error
		// (transport, timeout) is a real failure.
		var codeErr utilexec.CodeExitError
		if errors.As(streamErr, &codeErr) {
			exitCode = int32(codeErr.Code)
			message = fmt.Sprintf("exec_command exited with code %d in %s/%s container %q", exitCode, namespace, name, container)
		} else {
			return ExecutionResult{Status: StatusFailed, Message: fmt.Sprintf("exec stream failed: %v", streamErr)}
		}
	}

	// exit_code rides in the payloads map (like stdout/stderr) rather than as a
	// dedicated result field: no other action type produces one, so it does not
	// warrant a top-level schema field / DB column. The backend base64-decodes it.
	payloads := map[string][]byte{
		"exit_code": []byte(strconv.Itoa(int(exitCode))),
	}
	if stdout.Len() > 0 {
		payloads["stdout"] = stdout.Bytes()
	}
	if stderr.Len() > 0 {
		payloads["stderr"] = stderr.Bytes()
	}
	if stdout.truncated || stderr.truncated {
		message += " (output truncated)"
	}

	return ExecutionResult{
		Status:   StatusSuccess,
		Message:  message,
		Payloads: payloads,
	}
}

// buildRshellCommand assembles the rshell argv for the exec request.
func buildRshellCommand(binaryPath, script string, p effectivePolicy) []string {
	command := []string{binaryPath, "-c", script}
	if len(p.allowedPaths) > 0 {
		command = append(command, "-p", strings.Join(p.allowedPaths, ","))
	}
	if len(p.allowedCommands) > 0 {
		command = append(command, "--allowed-commands", strings.Join(p.allowedCommands, ","))
	}
	command = append(command, "--mode", p.mode)
	command = append(command, "--timeout", p.timeout.String())
	return command
}

// podHasContainer reports whether the pod defines a container with the given name.
func podHasContainer(pod *corev1.Pod, name string) bool {
	for _, c := range pod.Spec.Containers {
		if c.Name == name {
			return true
		}
	}
	return false
}

// cappedBuffer is an io.Writer that retains at most limit bytes and records whether
// any bytes were dropped. It never returns a short write, so the exec stream is not
// treated as errored when output exceeds the cap.
type cappedBuffer struct {
	data      []byte
	limit     int
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if remaining := b.limit - len(b.data); remaining > 0 {
		if len(p) > remaining {
			b.data = append(b.data, p[:remaining]...)
			b.truncated = true
		} else {
			b.data = append(b.data, p...)
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return len(p), nil
}

// Bytes returns the retained bytes.
func (b *cappedBuffer) Bytes() []byte { return b.data }

// Len returns the number of retained bytes.
func (b *cappedBuffer) Len() int { return len(b.data) }

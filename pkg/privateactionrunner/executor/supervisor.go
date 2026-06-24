// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	executorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
	"google.golang.org/grpc"
)

const (
	// defaultReadyTimeout bounds how long SubmitTask waits for a freshly spawned
	// executor to come up. A cold executor subprocess re-runs the full agent
	// bootstrap and fetches verification keys from remote config, which can take
	// well over 30s, so allow up to 90s before failing the first task.
	defaultReadyTimeout = 90 * time.Second
	statusPollInterval  = 200 * time.Millisecond
)

// Supervisor starts the executor process on demand and submits tasks to it.
type Supervisor struct {
	socketPath     string
	confPath       string
	extraConfFiles []string
	capacity       int32
	authToken      string
	command        command
	conn           *grpc.ClientConn
	client         executorpb.ExecutorClient

	// procCtx owns the spawned executor child's lifetime. It is deliberately
	// independent of the per-task submission context so that cancelling a submit
	// (or the polling loop) does not SIGKILL the child; the child is torn down
	// only by Close (procCancel), its own idle timeout, or a Drain-then-Close.
	procCtx    context.Context
	procCancel context.CancelFunc

	mu          sync.Mutex
	cmd         *exec.Cmd
	noAutoStart bool
}

type command struct {
	path        string
	baseArgs    []string
	dir         string
	description string
}

// NewSupervisor creates an executor supervisor.
func NewSupervisor(socketPath, confPath string, extraConfFiles []string, capacity int32, authToken string) *Supervisor {
	if capacity <= 0 {
		capacity = 1
	}
	conn, client, _ := newGRPCClient(socketPath, 5*time.Second)
	procCtx, procCancel := context.WithCancel(context.Background())
	return &Supervisor{
		socketPath:     socketPath,
		confPath:       confPath,
		extraConfFiles: append([]string(nil), extraConfFiles...),
		capacity:       capacity,
		authToken:      authToken,
		command:        defaultExecutorCommand(),
		conn:           conn,
		client:         client,
		procCtx:        procCtx,
		procCancel:     procCancel,
	}
}

// SetBinaryPath overrides the executor binary path. It is primarily useful for tests.
func (s *Supervisor) SetBinaryPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.command = command{path: path, baseArgs: []string{"executor"}, description: path + " executor"}
}

// WaitForCapacity blocks until a running executor has free capacity. If the executor
// is not running yet, it returns immediately so the orchestrator can dequeue the
// first task and start the executor lazily.
func (s *Supervisor) WaitForCapacity(ctx context.Context) error {
	if !s.isRunning() {
		return nil
	}
	ticker := time.NewTicker(statusPollInterval)
	defer ticker.Stop()
	for {
		status, err := s.Status(ctx)
		if err != nil {
			return nil
		}
		if status.Ready && s.capacity > status.ActiveTasks {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// SubmitTask ensures the executor is running, waits for capacity, and transfers
// accepted ownership of the task to the executor.
func (s *Supervisor) SubmitTask(ctx context.Context, task *types.Task) error {
	taskJSON := task.Raw
	if len(taskJSON) == 0 {
		var err error
		taskJSON, err = json.Marshal(task)
		if err != nil {
			return fmt.Errorf("marshal task: %w", err)
		}
	}
	if err := s.ensureRunning(ctx); err != nil {
		return err
	}
	if err := s.waitReady(ctx); err != nil {
		return err
	}
	for {
		resp, err := s.client.SubmitTask(withAuth(ctx, s.authToken), &executorpb.SubmitTaskRequest{TaskJson: taskJSON})
		if err == nil && resp.GetAccepted() {
			return nil
		}
		if err == nil {
			return fmt.Errorf("executor rejected task: %s", resp.GetReason())
		}
		if err != nil && !s.isRunning() {
			if startErr := s.ensureRunning(ctx); startErr != nil {
				return startErr
			}
		}
		select {
		case <-ctx.Done():
			if err != nil {
				return fmt.Errorf("submit task: %w", err)
			}
			return ctx.Err()
		case <-time.After(statusPollInterval):
		}
	}
}

// Status returns the executor's current status.
func (s *Supervisor) Status(ctx context.Context) (*executorpb.StatusResponse, error) {
	return s.client.Status(withAuth(ctx, s.authToken), &executorpb.StatusRequest{})
}

// Drain asks the executor to stop accepting new tasks and finish the ones it has
// already accepted. The server blocks until its in-flight tasks complete, so this
// call returns once the executor has drained or ctx's deadline elapses. It is
// best-effort: if no executor is listening, the error is returned for the caller
// to ignore.
func (s *Supervisor) Drain(ctx context.Context) error {
	_, err := s.client.Shutdown(withAuth(ctx, s.authToken), &executorpb.StatusRequest{})
	return err
}

// Close terminates any spawned executor child and releases the supervisor's
// client-side IPC connection.
func (s *Supervisor) Close() error {
	if s.procCancel != nil {
		s.procCancel()
	}
	if s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

// ShutdownExisting asks any executor already listening on this supervisor's
// socket to stop. It is best-effort so orchestrator startup can continue when no
// previous executor exists.
func (s *Supervisor) ShutdownExisting(ctx context.Context) {
	shutdownConn, shutdownClient, err := newGRPCClient(s.socketPath, 0)
	if err != nil {
		return
	}
	defer shutdownConn.Close()
	if _, err := shutdownClient.Shutdown(withAuth(ctx, s.authToken), &executorpb.StatusRequest{}); err != nil {
		return
	}

	ticker := time.NewTicker(statusPollInterval)
	defer ticker.Stop()
	for {
		if _, err := s.Status(ctx); err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ensureRunning starts the executor child if it is not already running. The
// child's lifetime is bound to the supervisor-owned procCtx, not to any caller
// context, so the parameter is intentionally unused.
func (s *Supervisor) ensureRunning(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.noAutoStart {
		return nil
	}
	if s.cmd != nil && s.cmd.Process != nil && s.cmd.ProcessState == nil {
		return nil
	}
	args := []string{"run", "--socket", s.socketPath}
	if s.confPath != "" {
		args = append(args, "--cfgpath", s.confPath)
	}
	for _, extra := range s.extraConfFiles {
		args = append(args, "--extracfgpath", extra)
	}
	commandArgs := append(append([]string(nil), s.command.baseArgs...), args...)
	// Bind the child to procCtx (supervisor-owned), not the submit ctx, so the
	// child outlives any single task submission and is only torn down by Close.
	cmd := exec.CommandContext(s.procCtx, s.command.path, commandArgs...)
	cmd.Dir = s.command.dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start executor %q: %w", s.command.description, err)
	}
	s.cmd = cmd
	go func() {
		_ = cmd.Wait()
		s.mu.Lock()
		if s.cmd == cmd {
			s.cmd = nil
		}
		s.mu.Unlock()
	}()
	return nil
}

func (s *Supervisor) waitReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, defaultReadyTimeout)
	defer cancel()
	ticker := time.NewTicker(statusPollInterval)
	defer ticker.Stop()
	for {
		status, err := s.Status(ctx)
		if err == nil && status.Ready && status.ProtocolVersion == ProtocolVersion {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("executor did not become ready: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (s *Supervisor) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cmd != nil && s.cmd.Process != nil && s.cmd.ProcessState == nil
}

func defaultExecutorCommand() command {
	exe, err := os.Executable()
	if err == nil {
		return command{path: exe, baseArgs: []string{"executor"}, description: exe + " executor"}
	}
	return command{
		path:        executableName("privateactionrunner"),
		baseArgs:    []string{"executor"},
		description: executableName("privateactionrunner") + " executor",
	}
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package binary holds the binary-mode executor implementation: it
// spawns a child agent process (re-exec'd into the hidden `executor
// run` subcommand) and forwards each task to it over a local gRPC
// socket.
package binary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/executor"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/shared/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/shared/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/shared/util"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	executorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Params configures the binary executor. The auth token is the agent's
// shared IPC session token (comp/core/ipc); the child reads the same
// token from disk so it does not traverse the command line.
type Params struct {
	SocketPath     string
	AuthToken      string
	DrainTimeout   time.Duration
	ConfPath       string
	ExtraConfFiles []string
}

// Executor forwards Execute calls to a child executor process over a
// local gRPC socket. The child runs the same agent binary with the
// hidden `executor run` subcommand.
//
// Lifetime contexts:
//   - procCtx owns the spawned child's lifetime. It is deliberately
//     independent of any per-call context so that cancelling a single
//     Execute RPC (or the polling loop) does not tear the child down.
//     Only Stop cancels it.
//   - The ctx passed to Execute bounds only the RPC call.
type Executor struct {
	socketPath   string
	authToken    string
	drainTimeout time.Duration

	confPath       string
	extraConfFiles []string

	binaryPath string
	binaryArgs []string

	procCtx    context.Context
	procCancel context.CancelFunc

	mu         sync.Mutex
	cmd        *exec.Cmd
	conn       *grpc.ClientConn
	client     executorpb.ExecutorClient
	clientOnce sync.Once
}

// New builds a binary-mode executor. AuthToken is required (callers
// pass the IPC component's session token).
func New(p Params) (*Executor, error) {
	if p.SocketPath == "" {
		p.SocketPath = executor.DefaultSocketPath()
	}
	if p.DrainTimeout <= 0 {
		p.DrainTimeout = 30 * time.Second
	}
	if p.AuthToken == "" {
		return nil, errors.New("binary executor requires a non-empty auth token (orchestrator should pass the IPC component's session token)")
	}
	binaryPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}

	procCtx, procCancel := context.WithCancel(context.Background())
	return &Executor{
		socketPath:     p.SocketPath,
		authToken:      p.AuthToken,
		drainTimeout:   p.DrainTimeout,
		confPath:       p.ConfPath,
		extraConfFiles: append([]string(nil), p.ExtraConfFiles...),
		binaryPath:     binaryPath,
		binaryArgs:     []string{"executor", "run"},
		procCtx:        procCtx,
		procCancel:     procCancel,
	}, nil
}

// SetBinary overrides the child binary command. Test-only.
func (e *Executor) SetBinary(path string, args []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.binaryPath = path
	e.binaryArgs = append([]string(nil), args...)
}

// Prepare dials the child. The child process is spawned lazily on the
// first Execute, so a runner with no work queued does not run the
// action surface at all.
func (e *Executor) Prepare(_ context.Context) error {
	var err error
	e.clientOnce.Do(func() {
		conn, dialErr := grpc.NewClient(
			executor.DialTarget(e.socketPath),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if dialErr != nil {
			err = fmt.Errorf("dial executor: %w", dialErr)
			return
		}
		e.conn = conn
		e.client = executorpb.NewExecutorClient(conn)
	})
	return err
}

// Execute marshals the task to JSON, ensures the child is running, and
// forwards the request. The ctx bounds only the RPC call — not the
// child's lifetime.
func (e *Executor) Execute(ctx context.Context, task *types.Task) (interface{}, error) {
	raw := task.Raw
	if len(raw) == 0 {
		marshaled, err := json.Marshal(task)
		if err != nil {
			return nil, fmt.Errorf("marshal task: %w", err)
		}
		raw = marshaled
	}
	if err := e.ensureRunning(ctx); err != nil {
		return nil, err
	}
	resp, err := e.client.Execute(executor.WithAuth(ctx, e.authToken), &executorpb.ExecuteRequest{TaskJson: raw})
	if err != nil {
		return nil, fmt.Errorf("execute via binary executor: %w", err)
	}
	if respErr := resp.GetError(); respErr != nil {
		return nil, util.PARError{
			ActionPlatformError: &aperrorpb.ActionPlatformError{
				ErrorCode:       aperrorpb.ActionPlatformErrorCode(respErr.GetCode()),
				Message:         respErr.GetMessage(),
				ExternalMessage: respErr.GetExternalMessage(),
			},
		}
	}
	if len(resp.GetOutputJson()) == 0 {
		return nil, nil
	}
	var output interface{}
	if err := json.Unmarshal(resp.GetOutputJson(), &output); err != nil {
		return nil, fmt.Errorf("decode executor response: %w", err)
	}
	return output, nil
}

// Stop tears the child down. Drain coordination is owned by the
// orchestrator (which waits for its own in-flight Execute calls before
// calling Stop); the binary executor's job here is just to cancel
// procCtx, which the child's cmd.Cancel translates into SIGTERM.
// WaitDelay enforces SIGKILL after a grace period.
func (e *Executor) Stop(ctx context.Context) error {
	e.procCancel()
	e.waitForExit(ctx)
	e.closeClient()
	return nil
}

func (e *Executor) ensureRunning(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cmd != nil && e.cmd.ProcessState == nil {
		return nil
	}
	cmd := exec.CommandContext(e.procCtx, e.binaryPath, e.childArgs()...)
	cmd.Env = os.Environ()
	cmd.Cancel = func() error {
		// Best-effort graceful signal to the child before WaitDelay
		// fires the hard kill.
		return executor.SignalProcess(cmd.Process)
	}
	cmd.WaitDelay = e.drainTimeout
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn executor: %w", err)
	}
	e.cmd = cmd

	go func() {
		err := cmd.Wait()
		e.mu.Lock()
		e.cmd = nil
		e.mu.Unlock()
		if err != nil && !errors.Is(e.procCtx.Err(), context.Canceled) {
			log.FromContext(ctx).Warn("executor child exited", log.ErrorField(err))
		}
	}()
	return nil
}

func (e *Executor) isRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cmd != nil
}

func (e *Executor) waitForExit(ctx context.Context) {
	deadline := time.Now().Add(e.drainTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	for time.Now().Before(deadline) {
		if !e.isRunning() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (e *Executor) closeClient() {
	e.mu.Lock()
	conn := e.conn
	e.conn = nil
	e.client = nil
	e.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

// childArgs returns the argv used to spawn the child. The IPC auth
// token is NOT passed on the command line — both processes read it
// independently from the shared IPC component.
func (e *Executor) childArgs() []string {
	args := append([]string(nil), e.binaryArgs...)
	args = append(args, "--socket-path", e.socketPath)
	if e.confPath != "" {
		args = append(args, "--cfgpath", e.confPath)
	}
	for _, f := range e.extraConfFiles {
		args = append(args, "--extra-config", f)
	}
	return args
}

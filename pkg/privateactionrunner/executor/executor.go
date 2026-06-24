// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package executor

import (
	"context"
	"fmt"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// Executor mode values, used as the value of the private_action_runner.executor_mode
// configuration setting.
const (
	// ModeInProcess (default) serves submitted tasks from a goroutine in the
	// orchestrator process. No subprocess is spawned.
	ModeInProcess = "in-process"
	// ModeBinary spawns the task executor as a child process, re-execing the
	// current binary (or the configured binary path).
	ModeBinary = "binary"
)

// TaskHandler is what an Executor needs from the orchestrator to serve tasks:
// one-time readiness preparation plus per-task handling. It is a superset of
// Handler (the narrower contract the IPC Server consumes).
type TaskHandler interface {
	Handler
	// Prepare readies the handler to process tasks (e.g. loads verification
	// keys). It must complete before the executor serves any task. Only the
	// implementation that runs the handler in this process calls it; the others
	// run the handler in a separate process that prepares itself.
	Prepare(ctx context.Context) error
}

// Executor routes accepted tasks to the process that runs them and manages that
// target's lifecycle. The orchestrator loop submits tasks through it; the
// concrete implementation decides where the task actually executes.
type Executor interface {
	// Start prepares the executor so SubmitTask can be called. The handler is
	// only used by the in-process implementation, which serves tasks from this
	// process; other implementations run the handler elsewhere and ignore it.
	Start(ctx context.Context, handler TaskHandler) error
	// WaitForCapacity blocks until the executor can accept another task.
	WaitForCapacity(ctx context.Context) error
	// SubmitTask transfers ownership of an accepted task to the executor.
	SubmitTask(ctx context.Context, task *types.Task) error
	// Stop releases any resources created by Start.
	Stop(ctx context.Context) error
}

// Params configures which executor implementation NewExecutor builds.
type Params struct {
	// Mode is one of ModeInProcess or ModeBinary. Empty defaults to ModeInProcess.
	Mode string
	// SocketPath is the local IPC socket or named pipe the orchestrator and
	// executor communicate over.
	SocketPath string
	// ConfPath and ExtraConfFiles are forwarded to the executor subprocess
	// (ModeBinary only).
	ConfPath       string
	ExtraConfFiles []string
	// Capacity is the maximum number of concurrent tasks the executor accepts.
	Capacity int32
	// AuthToken authenticates IPC requests between the orchestrator and executor.
	AuthToken string
	// Version is reported by the in-process server's status endpoint
	// (ModeInProcess only).
	Version string
	// OnShutdown is invoked when the in-process server stops (ModeInProcess only).
	OnShutdown func(string)
}

// NewExecutor builds the Executor implementation selected by p.Mode.
func NewExecutor(p Params) (Executor, error) {
	sup := NewSupervisor(p.SocketPath, p.ConfPath, p.ExtraConfFiles, p.Capacity, p.AuthToken)
	switch p.Mode {
	case ModeBinary:
		return newSubprocessExecutor(sup), nil
	case ModeInProcess, "":
		return newInProcessExecutor(sup, p.SocketPath, p.Version, p.AuthToken, p.OnShutdown), nil
	default:
		return nil, fmt.Errorf("unknown executor mode %q (want %q or %q)", p.Mode, ModeInProcess, ModeBinary)
	}
}

// subprocessExecutor runs the task executor as a child process. The supervisor
// spawns it lazily on the first task submission.
type subprocessExecutor struct {
	*Supervisor
}

func newSubprocessExecutor(sup *Supervisor) Executor {
	return &subprocessExecutor{Supervisor: sup}
}

func (e *subprocessExecutor) Start(ctx context.Context, _ TaskHandler) error {
	// Replace any executor left running by a previous orchestrator instance.
	e.ShutdownExisting(ctx)
	return nil
}

func (e *subprocessExecutor) Stop(ctx context.Context) error {
	// Drain first: let the executor finish tasks it has already accepted (bounded
	// by ctx's deadline) before Close tears the child down. Without this the child
	// would be killed with in-flight tasks still running, to be recovered only
	// later via lease expiry.
	_ = e.Supervisor.Drain(ctx)
	return e.Supervisor.Close()
}

// inProcessExecutor serves submitted tasks from a goroutine in the orchestrator
// process. Tasks still travel over the local IPC socket, so the submit path is
// identical to binary mode.
type inProcessExecutor struct {
	*Supervisor
	socketPath string
	version    string
	authToken  string
	onShutdown func(string)
	server     *Server
}

func newInProcessExecutor(sup *Supervisor, socketPath, version, authToken string, onShutdown func(string)) Executor {
	sup.noAutoStart = true
	return &inProcessExecutor{
		Supervisor: sup,
		socketPath: socketPath,
		version:    version,
		authToken:  authToken,
		onShutdown: onShutdown,
	}
}

func (e *inProcessExecutor) Start(ctx context.Context, handler TaskHandler) error {
	// The handler runs in this process, so ready it (load verification keys)
	// before we start serving tasks to it.
	if err := handler.Prepare(ctx); err != nil {
		return fmt.Errorf("prepare in-process executor handler: %w", err)
	}
	// Clear any stale executor from a previous run before we take the socket.
	e.ShutdownExisting(ctx)
	listener, err := Listen(e.socketPath)
	if err != nil {
		return fmt.Errorf("listen on in-process executor socket: %w", err)
	}
	// No idle timeout: the server's lifecycle is tied to the orchestrator process.
	e.server = NewServer(handler, e.version, 0, e.authToken, e.onShutdown)
	go func() {
		if err := e.server.Serve(ctx, listener); err != nil && e.onShutdown != nil {
			e.onShutdown(fmt.Sprintf("in-process executor server stopped with error: %v", err))
		}
	}()
	log.FromContext(ctx).Info("Started in-process executor on " + e.socketPath)
	return nil
}

func (e *inProcessExecutor) Stop(ctx context.Context) error {
	if e.server != nil {
		if err := e.server.Stop(ctx); err != nil {
			return err
		}
	}
	return e.Supervisor.Close()
}

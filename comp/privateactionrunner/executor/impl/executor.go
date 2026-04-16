// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

// Package impl is the execution-plane component for the PAR dual-process
// architecture.
//
// This is a stripped-down version of comp/privateactionrunner/impl.
// The control-plane responsibilities (OPMS polling, enrollment, health checks)
// have been removed — they move to the par-control Rust binary.
// What remains is identical to today: config loading, Remote Config key
// subscription, task signature verification, credential resolution, action
// allowlist enforcement, metrics, and action execution.
//
// IPC with par-control uses a binary length-framing protocol over UDS (see
// protocol.go).  This replaces the previous HTTP/1.1+JSON+base64 transport and
// eliminates the redundant encoding overhead for large task payloads.
package impl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	complog "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/actions"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	parconstants "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/parversion"
	pkgrcclient "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
	privatebundles "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles"
	credresolver "github.com/DataDog/datadog-agent/pkg/privateactionrunner/credentials/resolver"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
)

const (
	maxStartupWaitTimeout = 15 * time.Second
	gracefulDrainTimeout  = 30 * time.Second
)

// Params holds executor configuration that comes from CLI args, not datadog.yaml.
type Params struct {
	SocketPath         string
	IdleTimeoutSeconds int
}

// errorResponse is the JSON payload for binary error frames.
type errorResponse struct {
	ErrorCode    int32  `json:"error_code"`
	ErrorDetails string `json:"error_details"`
}

// Requires defines the FX dependencies for the executor component.
type Requires struct {
	Config    coreconfig.Component
	Log       complog.Component
	Lifecycle compdef.Lifecycle
	RcClient  rcclient.Component
	Params    Params
}

// Provides mirrors comp/privateactionrunner/impl.
type Provides struct {
	Comp privateactionrunner.Component
}

// ExecutorComponent is the stripped-down PAR component for the execution plane.
type ExecutorComponent struct {
	coreConfig model.ReaderWriter
	rcClient   pkgrcclient.Client
	logger     complog.Component

	params      Params
	started     bool
	startOnce   sync.Once
	startChan   chan struct{}
	cancelStart context.CancelFunc
	ln          net.Listener

	// connGroup tracks active connections for graceful drain on shutdown.
	connGroup sync.WaitGroup
	// lastActivity is updated at the END of each execute handler so the idle
	// timer does not fire while actions are in progress.
	lastActivity atomic.Int64
	// activeRequests counts goroutines currently executing an action.
	// The idle timer only fires when this reaches zero.
	activeRequests atomic.Int32
}

// NewComponent is the fxutil.ProvideComponentConstructor-compatible constructor.
func NewComponent(reqs Requires) (Provides, error) {
	ec := &ExecutorComponent{
		coreConfig: reqs.Config,
		rcClient:   pkgrcclient.NewAdapter(reqs.RcClient),
		logger:     reqs.Log,
		params:     reqs.Params,
		startChan:  make(chan struct{}),
	}
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: ec.Start,
		OnStop:  ec.Stop,
	})
	return Provides{Comp: ec}, nil
}

// Start implements privateactionrunner.Component.
func (ec *ExecutorComponent) Start(ctx context.Context) error {
	var err error
	ec.started = true
	ec.startOnce.Do(func() {
		defer close(ec.startChan)
		err = ec.start(ctx)
	})
	return err
}

// start is the real startup — mirrors PrivateActionRunner.start() exactly,
// replacing the WorkflowRunner+CommonRunner pair with a binary UDS server.
func (ec *ExecutorComponent) start(ctx context.Context) error {
	ctx, ec.cancelStart = context.WithCancel(ctx)
	defer ec.logger.Flush()

	// === Identical to PrivateActionRunner.start() ===
	cfg, err := ec.getRunnerConfig(ctx)
	if err != nil {
		ec.logger.Errorf("par-executor failed to start: %v", err)
		return err
	}
	commonTags := observability.CommonTags{
		RunnerId:      cfg.RunnerId,
		RunnerVersion: cfg.Version,
		Modes:         cfg.Modes,
		ExtraTags:     cfg.Tags,
	}
	ctx = observability.AddCommonTagsToLogs(ctx, commonTags)
	ec.logger.Info("par-executor starting")
	ec.logger.Info("==> Version : " + parversion.RunnerVersion)
	ec.logger.Info("==> Site : " + cfg.DatadogSite)
	ec.logger.Info("==> URN : " + cfg.Urn)

	keysManager := taskverifier.NewKeyManager(ec.rcClient)
	verifier := taskverifier.NewTaskVerifier(keysManager, cfg)

	startTime := time.Now()
	keysManager.Start(ctx)
	// === End identical section ===

	if os.Getenv(parconstants.InternalSkipTaskVerificationEnvVar) == "true" {
		ec.logger.Info("par-executor: task verification skipped (DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true)")
	} else {
		ec.logger.Info("par-executor: waiting for Remote Config signing keys")
		keysManager.WaitForReady()
		observability.ReportKeysManagerReady(cfg.MetricsClient, log.FromContext(ctx), startTime)
	}

	ec.ln, err = setupSocket(ec.params.SocketPath)
	if err != nil {
		return fmt.Errorf("par-executor: socket setup failed: %w", err)
	}

	// Concurrency semaphore — mirrors the original PAR RunnerPoolSize.
	poolSize := cfg.RunnerPoolSize
	if poolSize <= 0 {
		poolSize = 5
	}
	sem := make(chan struct{}, poolSize)
	ec.logger.Infof("par-executor: listening on %s (pool=%d, idle-timeout=%ds)",
		ec.params.SocketPath, poolSize, ec.params.IdleTimeoutSeconds)

	if ec.params.IdleTimeoutSeconds > 0 {
		ec.lastActivity.Store(time.Now().Unix())
		go ec.idleWatcher()
	}

	registry := privatebundles.NewRegistry(cfg, nil, nil)
	bundles := registry.Bundles
	resolver := credresolver.NewPrivateCredentialResolver()

	go ec.serveConnections(ctx, cfg, verifier, resolver, bundles, sem)
	return nil
}

// Stop implements privateactionrunner.Component.
// Closes the listener (no new connections) then waits for in-flight actions
// to complete before returning.
func (ec *ExecutorComponent) Stop(ctx context.Context) error {
	if !ec.started {
		return nil
	}
	ec.cancelStart()

	waitCtx, cancel := context.WithTimeout(ctx, maxStartupWaitTimeout)
	defer cancel()
	select {
	case <-ec.startChan:
	case <-waitCtx.Done():
		ec.logger.Warn("par-executor: startup did not complete before stop timeout")
	}

	if ec.ln != nil {
		ec.ln.Close()
	}

	// Drain in-flight connections with a hard deadline.
	done := make(chan struct{})
	go func() {
		ec.connGroup.Wait()
		close(done)
	}()
	drainCtx, drainCancel := context.WithTimeout(ctx, gracefulDrainTimeout)
	defer drainCancel()
	select {
	case <-done:
		ec.logger.Info("par-executor: all connections drained")
	case <-drainCtx.Done():
		ec.logger.Warn("par-executor: drain timeout reached, forcing shutdown")
	}

	os.Remove(ec.params.SocketPath)
	return nil
}

// serveConnections is the main accept loop.  Each connection is handled in its
// own goroutine.
func (ec *ExecutorComponent) serveConnections(
	ctx context.Context,
	cfg *parconfig.Config,
	verifier taskverifier.TaskVerifier,
	resolver credresolver.PrivateCredentialResolver,
	bundles map[string]types.Bundle,
	sem chan struct{},
) {
	for {
		conn, err := ec.ln.Accept()
		if err != nil {
			if isClosedNetErr(err) {
				return
			}
			ec.logger.Errorf("par-executor: accept error: %v", err)
			continue
		}
		ec.connGroup.Add(1)
		go func() {
			defer ec.connGroup.Done()
			ec.handleConn(ctx, conn, cfg, verifier, resolver, bundles, sem)
		}()
	}
}

// handleConn dispatches a single UDS connection.
// It enforces the SO_PEERCRED check, then routes by frame type.
func (ec *ExecutorComponent) handleConn(
	ctx context.Context,
	conn net.Conn,
	cfg *parconfig.Config,
	verifier taskverifier.TaskVerifier,
	resolver credresolver.PrivateCredentialResolver,
	bundles map[string]types.Bundle,
	sem chan struct{},
) {
	defer conn.Close()

	// Security: reject connections from processes running as a different UID.
	// This prevents an unprivileged process from injecting tasks into the
	// (potentially more-privileged) executor.
	logger := log.FromContext(ctx)

	if err := verifyCaller(conn); err != nil {
		logger.Errorf("par-executor: peer credential check failed: %v", err)
		return
	}

	var frameType [1]byte
	if _, err := io.ReadFull(conn, frameType[:]); err != nil {
		return
	}

	switch frameType[0] {
	case framePing:
		conn.Write([]byte{0x01}) //nolint:errcheck
	case frameExecute:
		ec.handleExecuteConn(ctx, conn, cfg, verifier, resolver, bundles, sem)
	default:
		logger.Errorf("par-executor: unknown frame type 0x%02x", frameType[0])
	}
}

// handleExecuteConn processes one execute frame.
// This implements the same logic as Loop.handleTask in workflow_executor.go,
// receiving the raw task bytes from par-control via the binary protocol instead
// of decoding a JSON+base64 HTTP request.
//
// Concurrency model mirrors the original PAR WorkflowRunner:
//   - sem limits concurrently running actions to RunnerPoolSize.
//   - activeRequests prevents the idle timer from firing during active work.
//   - lastActivity is updated at EXIT (not entry) so the idle clock starts only
//     after all in-flight work has completed.
func (ec *ExecutorComponent) handleExecuteConn(
	ctx context.Context,
	conn net.Conn,
	cfg *parconfig.Config,
	verifier taskverifier.TaskVerifier,
	resolver credresolver.PrivateCredentialResolver,
	bundles map[string]types.Bundle,
	sem chan struct{},
) {
	// Read frame: [4-byte task_len][task bytes][4-byte timeout_secs]
	rawBytes, timeoutSecs, err := readExecuteRequest(conn)
	if err != nil {
		log.FromContext(ctx).Errorf("par-executor: reading execute frame: %v", err)
		ec.sendError(conn, 0, fmt.Sprintf("malformed request: %v", err))
		return
	}

	// Acquire semaphore — blocks until a pool slot is free or ctx is cancelled.
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		ec.sendError(conn, 0, "executor shutting down")
		return
	}
	defer func() { <-sem }()

	ec.activeRequests.Add(1)
	defer func() {
		ec.activeRequests.Add(-1)
		if ec.params.IdleTimeoutSeconds > 0 {
			ec.lastActivity.Store(time.Now().Unix())
		}
	}()

	logger := log.FromContext(ctx)

	// Parse the raw OPMS task bytes — exactly one json.Unmarshal, same as the
	// original single-process PAR (no extra encoding overhead).
	var rawTask types.Task
	if err := json.Unmarshal(rawBytes, &rawTask); err != nil {
		ec.sendError(conn, 0, "invalid task JSON")
		return
	}
	rawTask.Raw = rawBytes

	// --- Identical to workflow_executor.go Loop.Run() from here ---

	if err := rawTask.Validate(); err != nil {
		logger.Errorf("par-executor: task validation failed: %v", err)
		parErr := util.DefaultPARError(err)
		ec.sendError(conn, int32(parErr.ErrorCode), parErr.Message)
		return
	}

	// NewTaskVerifier already returns a no-op verifier when
	// DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true, so no conditional here.
	task, err := verifier.UnwrapTask(&rawTask)
	if err != nil {
		logger.Errorf("par-executor: task verification failed: %v", err)
		parErr := util.DefaultPARError(err)
		ec.sendError(conn, int32(parErr.ErrorCode), parErr.Message)
		return
	}
	task.Data.Attributes.JobId = rawTask.Data.Attributes.JobId

	// Apply timeout.
	taskCtx := ctx
	if timeoutSecs > 0 {
		var cancel context.CancelFunc
		taskCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
		defer cancel()
	} else if cfg.TaskTimeoutSeconds != nil {
		var cancel context.CancelFunc
		taskCtx, cancel = context.WithTimeout(ctx, time.Duration(*cfg.TaskTimeoutSeconds)*time.Second)
		defer cancel()
	}

	var credential *privateconnection.PrivateCredentials
	if task.Data.Attributes.ConnectionInfo != nil {
		credential, err = resolver.ResolveConnectionInfoToCredential(taskCtx, task.Data.Attributes.ConnectionInfo, nil)
		if err != nil {
			logger.Errorf("par-executor: credential resolution failed: %v", err)
			parErr := util.DefaultPARError(err)
			ec.sendError(conn, int32(parErr.ErrorCode), parErr.Message)
			return
		}
	}

	// Heartbeats are sent by par-control while waiting for this response.
	output, err := runTask(taskCtx, cfg, bundles, task, credential)
	if err != nil {
		parErr := util.DefaultPARError(err)
		ec.sendError(conn, int32(parErr.ErrorCode), parErr.Message)
		return
	}

	// Send success response: raw JSON output bytes — no extra wrapping.
	outputJSON, err := json.Marshal(output)
	if err != nil {
		ec.sendError(conn, 0, fmt.Sprintf("failed to marshal output: %v", err))
		return
	}
	if err := writeOKResponse(conn, outputJSON); err != nil {
		logger.Warnf("par-executor: writing success response: %v", err)
	}
}

// sendError writes a binary error response to the connection.
func (ec *ExecutorComponent) sendError(conn net.Conn, code int32, details string) {
	payload, err := json.Marshal(errorResponse{ErrorCode: code, ErrorDetails: details})
	if err != nil {
		return
	}
	_ = writeErrorResponse(conn, payload) // best-effort; ignore write errors
}

// getRunnerConfig is identical to PrivateActionRunner.getRunnerConfig except
// that self-enrollment is never attempted (the control plane owns enrollment).
func (ec *ExecutorComponent) getRunnerConfig(ctx context.Context) (*parconfig.Config, error) {
	persistedIdentity, err := enrollment.GetIdentityFromPreviousEnrollment(ctx, ec.coreConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity: %w", err)
	}
	if persistedIdentity != nil {
		ec.coreConfig.Set(privateactionrunner.PARPrivateKey, persistedIdentity.PrivateKey, model.SourceAgentRuntime)
		ec.coreConfig.Set(privateactionrunner.PARUrn, persistedIdentity.URN, model.SourceAgentRuntime)
	}

	cfg, err := parconfig.FromDDConfig(ec.coreConfig)
	if err != nil {
		return nil, err
	}

	if cfg.IdentityIsIncomplete() {
		if os.Getenv(parconstants.InternalSkipTaskVerificationEnvVar) == "true" {
			return cfg, nil
		}
		return nil, errors.New("identity not found; provide a valid URN and private key in datadog.yaml")
	}
	return cfg, nil
}

// runTask mirrors WorkflowRunner.RunTask without the heartbeat goroutine.
func runTask(ctx context.Context, cfg *parconfig.Config, bundles map[string]types.Bundle, task *types.Task, credential *privateconnection.PrivateCredentials) (interface{}, error) {
	fqn := task.GetFQN()
	bundleName, actionName := actions.SplitFQN(fqn)

	bundle := bundles[bundleName]
	if bundle == nil {
		return nil, util.DefaultActionError(fmt.Errorf("unknown bundle: %s", bundleName))
	}
	action := bundle.GetAction(actionName)
	if action == nil {
		return nil, util.DefaultActionError(fmt.Errorf("unknown action %q in bundle %s", actionName, bundleName))
	}
	if !cfg.IsActionAllowed(bundleName, actionName) {
		return nil, util.DefaultActionError(fmt.Errorf("action %s is not in the allowlist", fqn))
	}

	logger := log.FromContext(ctx)
	startTime := observability.ReportExecutionStart(cfg.MetricsClient, task.Data.Attributes.Client, fqn, task.Data.ID, logger)
	output, err := action.Run(ctx, task, credential)
	observability.ReportExecutionCompleted(cfg.MetricsClient, task.Data.Attributes.Client, fqn, task.Data.ID, startTime, err, logger)

	if err != nil {
		return nil, util.DefaultActionError(err)
	}
	return output, nil
}

// idleWatcher calls os.Exit(0) after IdleTimeoutSeconds of inactivity.
// Only fires when activeRequests == 0 to avoid killing the executor mid-action.
func (ec *ExecutorComponent) idleWatcher() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	timeout := time.Duration(ec.params.IdleTimeoutSeconds) * time.Second
	for range ticker.C {
		if ec.activeRequests.Load() > 0 {
			continue
		}
		if idle := time.Since(time.Unix(ec.lastActivity.Load(), 0)); idle >= timeout {
			ec.logger.Infof("par-executor: idle for %v (no active requests), self-terminating", idle.Round(time.Second))
			os.Exit(0)
		}
	}
}

// setupSocket creates a Unix Domain Socket listener.
func setupSocket(socketPath string) (net.Listener, error) {
	if fi, err := os.Stat(socketPath); err == nil {
		if fi.Mode()&os.ModeSocket != 0 {
			os.Remove(socketPath)
		} else {
			return nil, fmt.Errorf("path %s exists and is not a socket", socketPath)
		}
	}
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("net.Listen unix %s: %w", socketPath, err)
	}
	os.Chmod(socketPath, 0720) //nolint:errcheck
	return ln, nil
}

// isClosedNetErr returns true when the error is from a closed listener.
func isClosedNetErr(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, net.ErrClosed)
}

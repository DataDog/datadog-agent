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
// The only new piece is the UDS HTTP server (same pattern as system-probe)
// which replaces the WorkflowRunner polling loop as the task intake point.
package impl

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	complog "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/actions"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/parversion"
	pkgrcclient "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
	com_datadoghq_remoteaction "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/remoteaction"
	com_datadoghq_remoteaction_rshell "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/remoteaction/rshell"
	credresolver "github.com/DataDog/datadog-agent/pkg/privateactionrunner/credentials/resolver"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
)

const (
	maxStartupWaitTimeout = 15 * time.Second
	// skipVerificationEnvVar bypasses task signature verification in e2e tests.
	skipVerificationEnvVar = "DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION"
)

// Params holds executor configuration that comes from CLI args, not datadog.yaml.
type Params struct {
	SocketPath         string
	IdleTimeoutSeconds int
}

// ExecuteRequest is the JSON body for POST /execute.
type ExecuteRequest struct {
	RawTask        string `json:"raw_task"`          // base64-encoded OPMS dequeue response
	TimeoutSeconds *int32 `json:"timeout_seconds,omitempty"`
}

// ExecuteResponse is the JSON body returned by POST /execute.
type ExecuteResponse struct {
	Output       interface{} `json:"output,omitempty"`
	ErrorCode    int32       `json:"error_code"`
	ErrorDetails string      `json:"error_details,omitempty"`
}

// Requires defines the FX dependencies — same as PrivateActionRunner.Requires
// minus Traceroute, EventPlatform, and Hostname (not needed for remote-actions
// and not needed since we don't do self-enrollment).
type Requires struct {
	Config    coreconfig.Component
	Log       complog.Component
	Lifecycle compdef.Lifecycle
	RcClient  rcclient.Component

	Params Params
}

// Provides mirrors comp/privateactionrunner/impl — needed for fxutil.ProvideComponentConstructor.
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

	// lastActivity is updated on each /execute request for idle detection.
	lastActivity atomic.Int64
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
// replacing the WorkflowRunner+CommonRunner pair with a UDS HTTP server.
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

	// Removed: opmsClient, workflowRunner.Start, commonRunner.Start

	// Wait for signing keys unless verification is being skipped for testing.
	if os.Getenv(skipVerificationEnvVar) == "true" {
		ec.logger.Info("par-executor: task verification skipped (DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true)")
	} else {
		ec.logger.Info("par-executor: waiting for Remote Config signing keys")
		keysManager.WaitForReady()
		observability.ReportKeysManagerReady(cfg.MetricsClient, log.FromContext(ctx), startTime)
	}

	// Open UDS server — same pattern as pkg/system-probe/api/server/listener_unix.go.
	ec.ln, err = setupSocket(ec.params.SocketPath)
	if err != nil {
		return fmt.Errorf("par-executor: socket setup failed: %w", err)
	}

	if ec.params.IdleTimeoutSeconds > 0 {
		ec.lastActivity.Store(time.Now().Unix())
		go ec.idleWatcher()
	}

	resolver := credresolver.NewPrivateCredentialResolver()
	bundles := map[string]types.Bundle{
		"com.datadoghq.remoteaction":        com_datadoghq_remoteaction.NewRemoteAction(),
		"com.datadoghq.remoteaction.rshell": com_datadoghq_remoteaction_rshell.NewRshellBundle(cfg.RShellAllowedPaths),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/execute", makeExecuteHandler(ctx, cfg, verifier, resolver, bundles, &ec.lastActivity, ec.params.IdleTimeoutSeconds))
	mux.HandleFunc("/debug/ready", handleReady)
	mux.HandleFunc("/debug/health", handleHealth)
	mux.HandleFunc("/debug/stats", handleStats)

	ec.logger.Infof("par-executor: listening on %s (idle-timeout=%ds)", ec.params.SocketPath, ec.params.IdleTimeoutSeconds)
	go func() {
		srv := &http.Server{Handler: mux}
		if serveErr := srv.Serve(ec.ln); serveErr != nil && !errors.Is(serveErr, net.ErrClosed) {
			ec.logger.Errorf("par-executor: server error: %v", serveErr)
		}
	}()

	return nil
}

// Stop implements privateactionrunner.Component.
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
		os.Remove(ec.params.SocketPath)
	}
	return nil
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
		if os.Getenv(skipVerificationEnvVar) == "true" {
			// Allow missing identity in e2e/POC testing — executor won't call OPMS
			// and won't verify signatures so URN/private-key are not needed.
			return cfg, nil
		}
		return nil, errors.New("identity not found; provide a valid URN and private key in datadog.yaml")
	}
	return cfg, nil
}

// makeExecuteHandler returns the POST /execute handler.
// This implements the same logic as Loop.handleTask in workflow_executor.go,
// but receives the raw task from par-control over UDS instead of from OPMS.
func makeExecuteHandler(
	baseCtx context.Context,
	cfg *parconfig.Config,
	verifier *taskverifier.TaskVerifier,
	resolver credresolver.PrivateCredentialResolver,
	bundles map[string]types.Bundle,
	lastActivity *atomic.Int64,
	idleTimeoutSeconds int,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if idleTimeoutSeconds > 0 {
			lastActivity.Store(time.Now().Unix())
		}

		ctx := r.Context()
		logger := log.FromContext(baseCtx)

		var req ExecuteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeExecuteError(w, http.StatusBadRequest, 0, "invalid request body")
			return
		}

		rawBytes, err := base64.StdEncoding.DecodeString(req.RawTask)
		if err != nil {
			writeExecuteError(w, http.StatusBadRequest, 0, "invalid base64 in raw_task")
			return
		}

		var rawTask types.Task
		if err := json.Unmarshal(rawBytes, &rawTask); err != nil {
			writeExecuteError(w, http.StatusBadRequest, 0, "invalid task JSON")
			return
		}
		rawTask.Raw = rawBytes

		// --- Identical to workflow_executor.go Loop.Run() task handling ---

		if err := rawTask.Validate(); err != nil {
			logger.Errorf("par-executor: task validation failed: %v", err)
			parErr := util.DefaultPARError(err)
			writeExecuteError(w, http.StatusUnprocessableEntity, int32(parErr.ErrorCode), parErr.Message)
			return
		}

		var task *types.Task
		if os.Getenv(skipVerificationEnvVar) == "true" {
			task, err = unwrapNoVerification(&rawTask)
		} else {
			task, err = verifier.UnwrapTaskFromSignedEnvelope(rawTask.Data.Attributes.SignedEnvelope)
		}
		if err != nil {
			logger.Errorf("par-executor: task verification failed: %v", err)
			parErr := util.DefaultPARError(err)
			writeExecuteError(w, http.StatusUnprocessableEntity, int32(parErr.ErrorCode), parErr.Message)
			return
		}
		// JobId is set by OPMS after signing; restore from the raw task.
		task.Data.Attributes.JobId = rawTask.Data.Attributes.JobId

		// Apply timeout (from request or config default).
		if req.TimeoutSeconds != nil && *req.TimeoutSeconds > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(*req.TimeoutSeconds)*time.Second)
			defer cancel()
		} else if cfg.TaskTimeoutSeconds != nil {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(*cfg.TaskTimeoutSeconds)*time.Second)
			defer cancel()
		}

		// Resolve credentials. ConnectionInfo is absent for actions like
		// testConnection that do not require a connection (e.g. no credentials).
		var credential *privateconnection.PrivateCredentials
		if task.Data.Attributes.ConnectionInfo != nil {
			credential, err = resolver.ResolveConnectionInfoToCredential(ctx, task.Data.Attributes.ConnectionInfo, nil)
			if err != nil {
				logger.Errorf("par-executor: credential resolution failed: %v", err)
				parErr := util.DefaultPARError(err)
				writeExecuteError(w, http.StatusUnprocessableEntity, int32(parErr.ErrorCode), parErr.Message)
				return
			}
		}

		// --- Run task (mirrors WorkflowRunner.RunTask minus heartbeat goroutine) ---
		// Heartbeats are sent by par-control while waiting for this response.
		output, err := runTask(ctx, cfg, bundles, task, credential)
		if err != nil {
			parErr := util.DefaultPARError(err)
			writeExecuteError(w, http.StatusOK, int32(parErr.ErrorCode), parErr.Message)
			return
		}

		writeJSON(w, http.StatusOK, ExecuteResponse{Output: output})
	}
}

// runTask mirrors WorkflowRunner.RunTask without the heartbeat goroutine.
// Heartbeats are sent by the Rust control plane while waiting for the /execute response.
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

// unwrapNoVerification extracts task fields from the signed envelope without
// verifying the signature. Used only when DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true.
func unwrapNoVerification(rawTask *types.Task) (*types.Task, error) {
	if rawTask.Data.Attributes == nil || rawTask.Data.Attributes.SignedEnvelope == nil {
		return nil, errors.New("task is missing signed_envelope")
	}
	env := rawTask.Data.Attributes.SignedEnvelope
	if len(env.Data) == 0 {
		return nil, errors.New("signed_envelope.data is empty")
	}
	var pb privateactionspb.PrivateActionTask
	if err := proto.Unmarshal(env.Data, &pb); err != nil {
		return nil, fmt.Errorf("failed to unmarshal PrivateActionTask: %w", err)
	}
	var inputs map[string]interface{}
	if pb.Inputs != nil {
		inputs = pb.Inputs.AsMap()
	}
	var t types.Task
	t.Data.ID = rawTask.Data.ID
	t.Data.Type = rawTask.Data.Type
	t.Data.Attributes = &types.Attributes{
		Name:           pb.ActionName,
		BundleID:       pb.BundleId,
		Client:         pb.Client,
		Inputs:         inputs,
		OrgId:          pb.OrgId,
		ConnectionInfo: pb.ConnectionInfo,
	}
	return &t, nil
}

// idleWatcher calls os.Exit(0) after IdleTimeoutSeconds of inactivity.
func (ec *ExecutorComponent) idleWatcher() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	timeout := time.Duration(ec.params.IdleTimeoutSeconds) * time.Second
	for range ticker.C {
		if idle := time.Since(time.Unix(ec.lastActivity.Load(), 0)); idle >= timeout {
			ec.logger.Infof("par-executor: idle for %v, self-terminating", idle.Round(time.Second))
			os.Exit(0)
		}
	}
}

// setupSocket mirrors pkg/system-probe/api/server/listener_unix.go.
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
	os.Chmod(socketPath, 0720)
	return ln, nil
}

func handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeExecuteError(w http.ResponseWriter, httpStatus int, errorCode int32, msg string) {
	writeJSON(w, httpStatus, ExecuteResponse{ErrorCode: errorCode, ErrorDetails: msg})
}

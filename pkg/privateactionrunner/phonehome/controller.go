// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package phonehome implements the opt-in core-Agent-owned PAR eligibility POC.
// It never enrolls, runs commands, stops PAR, or participates in Agent health.
package phonehome

import (
	"context"
	"crypto/ecdsa"
	"expvar"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	configenv "github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/fips"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

const (
	controlProcess  = "datadog-agent-par-control"
	executorProcess = "datadog-agent-action-executor"
	bootstrapBudget = 120 * time.Second
)

// State describes enrollment only, not action-execution readiness.
type State struct {
	State     string `json:"state"`
	Reason    string `json:"reason"`
	NextRetry string `json:"next_retry,omitempty"`
}

var status = struct {
	sync.RWMutex
	value *State
}{}

// Status returns a copy; nil means the POC has not been started.
func Status() *State {
	status.RLock()
	defer status.RUnlock()
	if status.value == nil {
		return nil
	}
	value := *status.value
	return &value
}

func publish(s State) {
	status.Lock()
	defer status.Unlock()
	status.value = &s
}

// Start is a no-op unless explicitly opted in. Config/secrets have already been
// resolved by core. Cancellation uses core's main context, not its startup deadline.
func Start(ctx context.Context, cfg model.Reader, hostname string) {
	if !enrollment.PhoneHomePOC() || !cfg.GetBool(setup.PAREnabled) {
		return
	}
	if expvar.Get("par_phone_home") == nil {
		expvar.Publish("par_phone_home", expvar.Func(func() interface{} { return Status() }))
	}
	buildFIPS, err := fips.Enabled()
	if runtime.GOOS != "linux" || configenv.IsContainerized() || flavor.GetFlavor() != flavor.DefaultAgent || err != nil || buildFIPS || cfg.GetBool("fips.enabled") ||
		len(cfg.GetStringMapString(setup.PAROpmsExtraHeaders)) != 0 ||
		!cfg.GetBool("private_action_runner.split_enabled") ||
		!filepath.IsAbs(cfg.GetString(setup.PARIdentityFilePath)) ||
		os.Getenv("DD_PRIVATE_ACTION_RUNNER_EXTRA_CONFIG_PATH") != "" {
		publish(State{State: "blocked", Reason: "unsupported_poc_configuration"})
		return
	}
	// Require an explicit socket: this must be an isolated supervisor, not an
	// accidentally selected host daemon. Unix socket permissions provide auth.
	socket := os.Getenv("DD_PM_SOCKET_PATH")
	if !filepath.IsAbs(socket) {
		publish(State{State: "blocked", Reason: "explicit_supervisor_socket_required"})
		return
	}
	endpoint := discoveryEndpoint(cfg)
	u, err := url.Parse(endpoint)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Host == "" ||
		(u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"))) {
		publish(State{State: "blocked", Reason: "unsafe_discovery_endpoint"})
		return
	}
	conn, err := grpc.NewClient("unix://"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		publish(State{State: "blocked", Reason: "supervisor_connection_failed"})
		return
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: httputils.CreateHTTPTransport(cfg),
		// DD-API-KEY is not one of net/http's automatically protected headers.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	c := &controller{cfg: cfg, hostname: hostname, client: client, endpoint: endpoint, pm: pb.NewProcessManagerClient(conn), emit: publish}
	go func() {
		defer conn.Close()
		defer client.CloseIdleConnections()
		c.run(ctx)
	}()
}

func discoveryEndpoint(cfg model.Reader) string {
	main := configutils.GetMainEndpoint(cfg, "https://api.", "dd_url")
	if os.Getenv(app.InternalUseDDURLForOPMSEnvVar) == "true" {
		return strings.TrimSuffix(main, "/")
	}
	site := configutils.ExtractSiteFromURL(main)
	if site == "" {
		site = "datadoghq.com"
	}
	return "https://api." + site
}

type controller struct {
	// step is serialized even in tests; production has just one loop.
	mu            sync.Mutex
	cfg           model.Reader
	hostname      string
	client        *http.Client
	endpoint      string
	pm            pb.ProcessManagerClient
	emit          func(State)
	nextDiscovery time.Time
	lastDiscovery discovery
	backoff       time.Duration
	started       time.Time
}

func (c *controller) run(ctx context.Context) {
	// Jitter initial discovery too, to avoid synchronized Agent starts.
	delay := time.Duration(rand.Int64N(int64(pollInterval)))
	c.emit(State{State: "waiting", Reason: "startup_jitter", NextRetry: time.Now().Add(delay).UTC().Format(time.RFC3339)})
	for {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		delay = c.step(ctx, time.Now())
		if delay == 0 {
			return
		}
	}
}

func (c *controller) step(ctx context.Context, now time.Time) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil || !c.cfg.GetBool(setup.PAREnabled) {
		return 0
	}
	set := func(state, reason string, delay time.Duration) time.Duration {
		s := State{State: state, Reason: reason}
		if delay > 0 {
			s.NextRetry = now.Add(delay).UTC().Format(time.RFC3339)
		}
		c.emit(s)
		return delay
	}
	identity, err := enrollment.GetIdentityFromPreviousEnrollment(ctx, c.cfg)
	if err != nil {
		return set("blocked", "identity_unreadable", slowRetry)
	}
	// Persisted identity has precedence over inline identity, as in PAR itself.
	knownIdentity := identity != nil
	reusable := false
	if identity != nil {
		reusable = !enrollment.ShouldReenroll(&enrollment.AgentIdentifier{Hostname: c.hostname}, identity, c.cfg.GetString("api_key"))
	}
	// PAR falls back to its configured identity when discarding a saved one.
	if !reusable && c.cfg.GetString(setup.PARUrn) != "" && c.cfg.GetString(setup.PARPrivateKey) != "" {
		identity = &enrollment.PersistedIdentity{URN: c.cfg.GetString(setup.PARUrn), PrivateKey: c.cfg.GetString(setup.PARPrivateKey)}
		knownIdentity, reusable = true, true
	}
	if reusable {
		key, keyErr := util.Base64ToJWK(identity.PrivateKey)
		_, urnErr := util.ParseRunnerURN(identity.URN)
		_, private := key.Key.(*ecdsa.PrivateKey)
		if keyErr != nil || urnErr != nil || !private || !key.Valid() {
			return set("blocked", "identity_invalid", slowRetry)
		}
	}
	// Inspect both registered processes; shipping auto-start/restart policies
	// cannot be safely combined with this gate. Never rewrite them at runtime.
	rpcCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	control, err := c.pm.Describe(rpcCtx, &pb.DescribeRequest{NameOrUuid: controlProcess})
	if err != nil || control.GetDetail() == nil {
		return set("blocked", "supervisor_unavailable", slowRetry)
	}
	executor, err := c.pm.Describe(rpcCtx, &pb.DescribeRequest{NameOrUuid: executorProcess})
	if err != nil || executor.GetDetail() == nil {
		return set("blocked", "executor_not_registered", slowRetry)
	}
	for _, p := range []*pb.ProcessDetail{control.GetDetail(), executor.GetDetail()} {
		if p.AutoStart || p.RestartPolicy != "never" {
			return set("blocked", "supervisor_must_be_on_demand_no_restart", slowRetry)
		}
	}
	if executor.GetDetail().Env[app.PhoneHomePOCEnvVar] != "true" {
		return set("blocked", "executor_must_opt_into_poc_guard", slowRetry)
	}
	active := func(p *pb.ProcessDetail) bool {
		return p.State == pb.ProcessState_RUNNING || p.State == pb.ProcessState_STARTING || p.State == pb.ProcessState_STOPPING
	}
	if reusable && active(control.GetDetail()) {
		// A confirmed identity supersedes the attempt latch. Leave no stale
		// latch to block the existing hostname/key re-enrollment on a later
		// joint core/PAR restart. Never remove unknown files from this directory.
		attempt := enrollment.PhoneHomeAttemptPath(c.cfg)
		for _, name := range []string{"post", "outcome", ""} {
			if err := os.Remove(filepath.Join(attempt, name)); err != nil && !os.IsNotExist(err) {
				return set("enrolled", "identity_persisted_attempt_cleanup_failed", 0)
			}
		}
		return set("enrolled", "identity_reused_or_persisted_not_action_readiness", 0)
	}

	attempt := enrollment.PhoneHomeAttemptPath(c.cfg)
	info, statErr := os.Stat(attempt)
	if statErr != nil && !os.IsNotExist(statErr) {
		return set("blocked", "attempt_journal_unreadable", slowRetry)
	}
	attempted := statErr == nil
	if attempted && !reusable {
		// Keep discovery alive, at a slow rate after failure, but never repeat
		// the launch/POST without operator recovery of this durable latch.
		if c.started.IsZero() {
			c.started = info.ModTime()
		}
		reason := "awaiting_persisted_identity"
		state, delay := "launching", time.Second
		outcome, readErr := os.ReadFile(filepath.Join(attempt, "outcome"))
		if readErr == nil || now.Sub(c.started) >= bootstrapBudget {
			state, delay, reason = "blocked", slowRetry, "enrollment_outcome_unknown_operator_recovery_required"
			switch string(outcome) {
			case "enrollment_rejected_not_necessarily_scope", "enrollment_ambiguous_or_persistence_failed":
				reason = string(outcome)
			}
		}
		if !knownIdentity && !now.Before(c.nextDiscovery) {
			c.discover(ctx, now, delay)
		}
		return set(state, reason, delay)
	}
	if active(control.GetDetail()) || active(executor.GetDetail()) {
		return set("blocked", "adopting_existing_process_without_identity", slowRetry)
	}
	if !knownIdentity {
		if !c.cfg.GetBool(setup.PARSelfEnroll) || !c.cfg.GetBool(setup.PARApiKeyOnlyEnrollment) {
			return set("blocked", "fresh_poc_requires_api_key_self_enrollment", slowRetry)
		}
		if !now.Before(c.nextDiscovery) {
			c.discover(ctx, now, 0)
		}
		if !c.lastDiscovery.eligible {
			state := "blocked"
			if c.lastDiscovery.reason == "missing_enrollment_scope" {
				state = "waiting"
			}
			return set(state, c.lastDiscovery.reason, max(time.Second, c.nextDiscovery.Sub(now)))
		}
	}
	if reusable && !c.started.IsZero() {
		return set("blocked", "enrolled_process_exited_operator_recovery_required", slowRetry)
	}
	if ctx.Err() != nil || !c.cfg.GetBool(setup.PAREnabled) {
		return 0
	}
	// Existing identities bypass discovery even if PAR decides to re-enroll.
	// A reusable identity needs no mutation latch on subsequent core restarts.
	if !reusable {
		if err := os.Mkdir(attempt, 0700); err != nil {
			return set("blocked", "attempt_already_reserved_or_unwritable", slowRetry)
		}
	}
	c.started = now
	startCtx, startCancel := context.WithTimeout(ctx, 5*time.Second)
	defer startCancel()
	_, err = c.pm.Start(startCtx, &pb.StartRequest{NameOrUuid: control.GetDetail().Uuid})
	if err != nil {
		return set("blocked", "launch_outcome_unknown", slowRetry)
	}
	return set("launching", "awaiting_persisted_identity", time.Second)
}

func (c *controller) discover(ctx context.Context, now time.Time, floor time.Duration) {
	result := validate(ctx, c.client, c.endpoint, c.cfg.GetString("api_key"))
	if result.transient {
		c.backoff = min(slowRetry, max(pollInterval, c.backoff*2))
		result.delay = c.backoff
	} else {
		c.backoff = 0
	}
	result.delay = max(result.delay, floor)
	// Positive jitter preserves Retry-After as a lower bound (including large hints).
	jitter := time.Duration(rand.Int64N(int64(pollInterval / 5)))
	if result.delay <= slowRetry {
		result.delay += jitter
	}
	c.nextDiscovery, c.lastDiscovery = now.Add(result.delay), result
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package phonehome implements the opt-in core-owned PAR eligibility controller.
// Enrollment and credential persistence remain in the Go executor.
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

// State contains no identity, response body or credential. Action readiness is
// intentionally not inferred from scope, a PID, or persisted enrollment.
type State struct {
	State                  string `json:"state"`
	Reason                 string `json:"reason"`
	NextRetry              string `json:"next_retry,omitempty"`
	ScopeReady             bool   `json:"scope_ready"`
	ScopeChecked           bool   `json:"scope_checked"`
	EnrollmentOutcome      string `json:"enrollment_outcome"`
	RetrySafe              bool   `json:"retry_safe"`
	ReconciliationRequired bool   `json:"reconciliation_required"`
	NextDiscovery          string `json:"next_discovery,omitempty"`
	NextEnrollmentAttempt  string `json:"next_enrollment_attempt,omitempty"`
	EnrollmentRetryAfter   string `json:"enrollment_retry_after,omitempty"`
	HelperRetained         bool   `json:"helper_retained"`
}

var status = struct {
	sync.RWMutex
	value *State
}{}

func Status() *State {
	status.RLock()
	defer status.RUnlock()
	if status.value == nil {
		return nil
	}
	value := *status.value
	return &value
}
func publish(s State) { status.Lock(); defer status.Unlock(); status.value = &s }

// Start is asynchronous and independent of RC readiness and core health.
func Start(ctx context.Context, cfg model.Reader, hostname string) {
	if !enrollment.PhoneHomePOC() || !cfg.GetBool(setup.PAREnabled) {
		return
	}
	if expvar.Get("par_phone_home") == nil {
		expvar.Publish("par_phone_home", expvar.Func(func() interface{} { return Status() }))
	}
	buildFIPS, err := fips.Enabled()
	if runtime.GOOS != "linux" || configenv.IsContainerized() || flavor.GetFlavor() != flavor.DefaultAgent || err != nil || buildFIPS || cfg.GetBool("fips.enabled") ||
		len(cfg.GetStringMapString(setup.PAROpmsExtraHeaders)) != 0 || !cfg.GetBool("private_action_runner.split_enabled") ||
		!filepath.IsAbs(cfg.GetString(setup.PARIdentityFilePath)) || os.Getenv("DD_PRIVATE_ACTION_RUNNER_EXTRA_CONFIG_PATH") != "" {
		publish(State{State: "blocked", Reason: "unsupported_poc_configuration"})
		return
	}
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
	client := &http.Client{Timeout: 10 * time.Second, Transport: httputils.CreateHTTPTransport(cfg),
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	c := &controller{cfg: cfg, hostname: hostname, client: client, endpoint: endpoint, pm: pb.NewProcessManagerClient(conn), emit: publish}
	go func() { defer conn.Close(); defer client.CloseIdleConnections(); c.run(ctx) }()
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
	mu            sync.Mutex
	cfg           model.Reader
	hostname      string
	client        *http.Client
	endpoint      string
	pm            pb.ProcessManagerClient
	emit          func(State)
	jitter        func(time.Duration) time.Duration
	nextDiscovery time.Time
	lastDiscovery discovery
	backoff       time.Duration
	configHash    string
	apiKey        string // in-memory startup snapshot; never persisted or logged
	started       time.Time
}

func (c *controller) random(limit time.Duration) time.Duration {
	if c.jitter != nil {
		return c.jitter(limit)
	}
	return time.Duration(rand.Int64N(int64(limit)))
}
func timestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (c *controller) run(ctx context.Context) {
	delay := c.random(pollInterval)
	c.emit(State{State: "waiting", Reason: "startup_jitter", NextDiscovery: timestamp(time.Now().Add(delay))})
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

func active(p *pb.ProcessDetail) bool {
	return p.State == pb.ProcessState_RUNNING || p.State == pb.ProcessState_STARTING || p.State == pb.ProcessState_STOPPING
}

func (c *controller) step(ctx context.Context, now time.Time) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil || !c.cfg.GetBool(setup.PAREnabled) {
		return 0
	}
	state := State{EnrollmentOutcome: "not_attempted"}
	set := func(name, reason string, delay time.Duration) time.Duration {
		state.State, state.Reason = name, reason
		state.ScopeReady, state.NextDiscovery = c.lastDiscovery.eligible, timestamp(c.nextDiscovery)
		state.ScopeChecked = c.lastDiscovery.reason != ""
		if delay > 0 {
			state.NextRetry = timestamp(now.Add(delay))
		}
		c.emit(state)
		return delay
	}
	hash := enrollment.PhoneHomeConfigHash(c.cfg, c.hostname)
	if c.configHash == "" {
		c.configHash = hash
		c.apiKey = c.cfg.GetString("api_key")
	}
	if hash != c.configHash {
		return set("blocked", "config_changed_restart_required", slowRetry)
	}
	a, err := enrollment.ReadPhoneHomeAttempt(c.cfg)
	if err != nil {
		state.ReconciliationRequired = true
		return set("blocked", "journal_unreadable_reconciliation_required", slowRetry)
	}
	var outcome *enrollment.PhoneHomeOutcome
	if a != nil {
		outcome, err = enrollment.ReadPhoneHomeOutcome(c.cfg, a)
		if err != nil {
			state.ReconciliationRequired = true
			return set("blocked", "outcome_unreadable", slowRetry)
		}
		state.EnrollmentOutcome = "pending"
		if outcome != nil {
			state.EnrollmentOutcome, state.RetrySafe = outcome.Category, outcome.RetrySafe
			state.EnrollmentRetryAfter = timestamp(outcome.RetryAt)
			state.ReconciliationRequired, state.HelperRetained = outcome.ReconciliationRequired, outcome.HelperRetained
		}
	}
	identity, identityErr := enrollment.GetIdentityFromPreviousEnrollment(ctx, c.cfg)
	known := identity != nil
	reusable := known && !enrollment.ShouldReenroll(&enrollment.AgentIdentifier{Hostname: c.hostname}, identity, c.cfg.GetString("api_key"))
	if identityErr == nil && !reusable && c.cfg.GetString(setup.PARUrn) != "" && c.cfg.GetString(setup.PARPrivateKey) != "" {
		identity = &enrollment.PersistedIdentity{URN: c.cfg.GetString(setup.PARUrn), PrivateKey: c.cfg.GetString(setup.PARPrivateKey)}
		known, reusable = true, true
	}
	if reusable {
		key, keyErr := util.Base64ToJWK(identity.PrivateKey)
		_, urnErr := util.ParseRunnerURN(identity.URN)
		_, private := key.Key.(*ecdsa.PrivateKey)
		if keyErr != nil || urnErr != nil || !private || !key.Valid() {
			return set("blocked", "identity_invalid", slowRetry)
		}
	}
	if identityErr != nil && (outcome == nil || !outcome.PersistencePending) {
		return set("blocked", "identity_unreadable", slowRetry)
	}
	if a != nil && (outcome == nil || outcome.Category != "identity_persisted") {
		// The file can become visible before fsync/outcome publication. For an
		// owned attempt, require Go's durable-persistence acknowledgement too.
		reusable = false
	}
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
	for _, p := range []*pb.ProcessDetail{control.Detail, executor.Detail} {
		if p.AutoStart || p.RestartPolicy != "never" {
			return set("blocked", "supervisor_must_be_on_demand_no_restart", slowRetry)
		}
	}
	if executor.Detail.Env[app.PhoneHomePOCEnvVar] != "true" {
		return set("blocked", "executor_must_opt_into_poc_guard", slowRetry)
	}
	if reusable && active(control.Detail) {
		state.EnrollmentOutcome, state.ReconciliationRequired, state.HelperRetained = "enrolled", false, false
		if a != nil {
			if err := enrollment.CompletePhoneHomeAttempt(c.cfg, a); err != nil {
				return set("enrolled", "journal_cleanup_failed", 0)
			}
		}
		c.nextDiscovery = time.Time{}
		return set("enrolled", "identity_confirmed_not_action_readiness", 0)
	}

	if a == nil && !known && (!c.cfg.GetBool(setup.PARSelfEnroll) || !c.cfg.GetBool(setup.PARApiKeyOnlyEnrollment)) {
		return set("blocked", "fresh_poc_requires_api_key_self_enrollment", slowRetry)
	}
	// Polling may report scope readiness during cooldown, but can never change
	// the durable enrollment deadline. Existing identity startup bypasses GET.
	if !reusable && (!known || (a != nil && !a.PreviouslyEnrolled) || (outcome != nil && outcome.RetrySafe)) && !now.Before(c.nextDiscovery) {
		c.discover(ctx, now)
	}
	if a != nil && !reusable {
		if outcome != nil && outcome.HelperRetained {
			if active(executor.Detail) {
				return set("blocked", "persistence_memory_helper_retained", time.Minute)
			}
			state.HelperRetained, state.ReconciliationRequired = false, true
			state.EnrollmentOutcome = "pending_identity_holder_lost"
			lost := *outcome
			lost.PersistencePending, lost.HelperRetained, lost.RetrySafe = false, false, false
			outcome = &lost // Cannot reconstruct a returned URN from a prepare-only record.
		}
		// A recovery launch uses the SAME attempt/key, not another POST. While
		// it is active, its old persistence outcome must not trigger Stop.
		if a.Recovering && (active(control.Detail) || active(executor.Detail)) && now.Sub(a.StartedAt) < bootstrapBudget {
			return set("launching", "recovering_persisted_identity", time.Second)
		}
		if outcome == nil && now.Sub(a.StartedAt) < bootstrapBudget {
			return set("launching", "awaiting_enrollment_outcome", time.Second)
		}
		if outcome == nil {
			state.ReconciliationRequired = true
			state.EnrollmentOutcome = "interrupted_attempt"
		}
		canRetry := outcome != nil && (outcome.RetrySafe || outcome.PersistencePending)
		if canRetry && a.NextAttemptAt.IsZero() {
			base, capDelay, number := 5*time.Minute, 30*time.Minute, a.Number
			if outcome.PersistencePending {
				base, capDelay, number = time.Minute, 15*time.Minute, a.RecoveryCount+1
			}
			for i := 1; i < number && base < capDelay; i++ {
				base = min(capDelay, base*2)
			}
			a.NextAttemptAt = now.Add(base + c.random(base/5))
			if outcome.RetryAt.After(a.NextAttemptAt) {
				a.NextAttemptAt = outcome.RetryAt
			}
			if err := enrollment.SavePhoneHomeSchedule(c.cfg, a); err != nil {
				return set("blocked", "cooldown_persistence_failed", slowRetry)
			}
			if outcome.Category == "missing_scope" || outcome.Category == "invalid_credentials" {
				// POST is authoritative; invalidate preflight until a fresh check.
				c.lastDiscovery.eligible = false
				c.nextDiscovery = now
			}
		}
		state.NextEnrollmentAttempt = timestamp(a.NextAttemptAt)
		if outcome == nil && active(executor.Detail) {
			// An unwritable journal may hide an in-memory credential holder.
			// Do not kill it merely because the bootstrap deadline has elapsed.
			state.HelperRetained, state.ReconciliationRequired = true, true
			return set("blocked", "unconfirmed_executor_may_hold_identity", time.Minute)
		}
		// Only failed, owned startup attempts reach here. A healthy enrolled PAR
		// returned above. No mutation retries can bypass quiescence/cooldown.
		if active(control.Detail) || active(executor.Detail) {
			for _, p := range []*pb.ProcessDetail{control.Detail, executor.Detail} {
				if !active(p) || p.State == pb.ProcessState_STOPPING {
					continue
				}
				stopCtx, stopCancel := context.WithTimeout(ctx, 5*time.Second)
				_, stopErr := c.pm.Stop(stopCtx, &pb.StopRequest{NameOrUuid: p.Uuid})
				stopCancel()
				if stopErr != nil {
					return set("blocked", "failed_startup_stop_pending", time.Minute)
				}
			}
			return set("blocked", "quiescing_failed_startup", time.Second)
		}
		if !canRetry {
			return set("blocked", state.EnrollmentOutcome, time.Minute)
		}
		if now.Before(a.NextAttemptAt) {
			return set("blocked", state.EnrollmentOutcome, min(time.Minute, a.NextAttemptAt.Sub(now)))
		}
		if outcome.PersistencePending {
			a.Recovering, a.StartedAt = true, now
			a.RecoveryCount++
			a.NextAttemptAt = time.Time{} // next failure gets its own durable cooldown
			if err := enrollment.SavePhoneHomeSchedule(c.cfg, a); err != nil {
				return set("blocked", "recovery_schedule_failed", slowRetry)
			}
			return c.launch(ctx, now, control.Detail.Uuid, set)
		}
		// Rejections authorize another attempt only after current validation.
		if !c.lastDiscovery.eligible {
			return set("waiting", c.lastDiscovery.reason, max(time.Second, c.nextDiscovery.Sub(now)))
		}
	}
	if active(control.Detail) || active(executor.Detail) {
		return set("blocked", "adopting_existing_process", time.Minute)
	}
	if a == nil && !known {
		if !c.lastDiscovery.eligible {
			name := "blocked"
			if c.lastDiscovery.reason == "missing_enrollment_scope" {
				name = "waiting"
			}
			return set(name, c.lastDiscovery.reason, max(time.Second, c.nextDiscovery.Sub(now)))
		}
	}
	if ctx.Err() != nil || !c.cfg.GetBool(setup.PAREnabled) {
		return 0
	}
	if enrollment.PhoneHomeConfigHash(c.cfg, c.hostname) != c.configHash {
		return set("blocked", "config_changed_restart_required", slowRetry)
	}
	if !reusable {
		if _, err := enrollment.ReservePhoneHomeAttempt(c.cfg, c.hostname, now, a); err != nil {
			return set("blocked", "attempt_reservation_failed", slowRetry)
		}
		state.EnrollmentOutcome, state.NextEnrollmentAttempt = "pending", ""
		state.RetrySafe, state.ReconciliationRequired = false, false
	} else if !c.started.IsZero() {
		return set("blocked", "enrolled_process_exited", slowRetry)
	}
	return c.launch(ctx, now, control.Detail.Uuid, set)
}

func (c *controller) launch(ctx context.Context, now time.Time, uuid string, set func(string, string, time.Duration) time.Duration) time.Duration {
	c.started = now
	startCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := c.pm.Start(startCtx, &pb.StartRequest{NameOrUuid: uuid}); err != nil {
		return set("blocked", "launch_outcome_unknown", time.Minute)
	}
	return set("launching", "awaiting_persisted_identity", time.Second)
}

func (c *controller) discover(ctx context.Context, now time.Time) {
	result := validate(ctx, c.client, c.endpoint, c.apiKey)
	if result.transient {
		c.backoff = min(slowRetry, max(pollInterval, c.backoff*2))
		result.delay = c.backoff
	} else {
		c.backoff = 0
	}
	if result.delay <= slowRetry {
		result.delay += c.random(pollInterval / 5)
	}
	c.nextDiscovery, c.lastDiscovery = now.Add(result.delay), result
}

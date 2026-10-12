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
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	phonehomedef "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/def"
	configenv "github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/fips"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

const (
	controlProcess  = "datadog-agent-par-control"
	executorProcess = "datadog-agent-action-executor"
	bootstrapBudget = 120 * time.Second
	pollInterval    = time.Minute
	slowRetry       = 15 * time.Minute
)

// State contains no identity, response body or credential. Action readiness is
// intentionally not inferred from scope, a PID, or persisted enrollment.
type State = phonehomedef.State

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

// Run is owned by the component's background lifecycle, never core startup.
func Run(ctx context.Context, cfg model.Reader, hostname string, source Source) {
	if !enrollment.PhoneHomePOC() || !cfg.GetBool(par.Enabled) {
		return
	}
	if expvar.Get("par_phone_home") == nil {
		expvar.Publish("par_phone_home", expvar.Func(func() interface{} { return Status() }))
	}
	buildFIPS, err := fips.Enabled()
	if runtime.GOOS != "linux" || configenv.IsContainerized() || flavor.GetFlavor() != flavor.DefaultAgent || err != nil || buildFIPS || cfg.GetBool("fips.enabled") ||
		len(cfg.GetStringMapString(par.OPMSExtraHeaders)) != 0 || !cfg.GetBool(par.SplitEnabled) ||
		!filepath.IsAbs(cfg.GetString(par.IdentityFilePath)) || os.Getenv("DD_PRIVATE_ACTION_RUNNER_EXTRA_CONFIG_PATH") != "" {
		publish(State{State: "blocked", Reason: "unsupported_poc_configuration"})
		return
	}
	socket := os.Getenv("DD_PM_SOCKET_PATH")
	if !filepath.IsAbs(socket) {
		publish(State{State: "blocked", Reason: "explicit_supervisor_socket_required"})
		return
	}
	conn, err := grpc.NewClient("unix://"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		publish(State{State: "blocked", Reason: "supervisor_connection_failed"})
		return
	}
	if source == nil {
		source = NewSource(cfg, nil)
	}
	c := &controller{cfg: cfg, hostname: hostname, source: source, now: time.Now, pm: pb.NewProcessManagerClient(conn), emit: publish}
	defer conn.Close()
	c.run(ctx)
}

type controller struct {
	mu          sync.Mutex
	cfg         model.Reader
	hostname    string
	source      Source
	now         func() time.Time
	observation Prerequisites
	pm          pb.ProcessManagerClient
	emit        func(State)
	jitter      func(time.Duration) time.Duration
	configHash  string
	started     time.Time
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
	defer c.source.Demand(false)
	delay := c.random(pollInterval)
	c.emit(State{State: "waiting", Reason: "startup_jitter", NextRetry: timestamp(time.Now().Add(delay))})
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
	if ctx.Err() != nil || !c.cfg.GetBool(par.Enabled) {
		c.source.Demand(false)
		return 0
	}
	c.observation = c.source.Read(now)
	if !c.observation.RCReady {
		c.source.Demand(false)
	}
	state := State{EnrollmentOutcome: "not_attempted"}
	set := func(name, reason string, delay time.Duration) time.Duration {
		state.State, state.Reason = name, reason
		state.RCReady, state.RCReason = c.observation.RCReady, c.observation.RCReason
		state.ScopeReady, state.NextDiscovery = c.observation.ScopeReady, timestamp(c.observation.NextCheck)
		state.ScopeChecked = !c.observation.ScopeCheckedAt.IsZero()
		if delay > 0 {
			state.NextRetry = timestamp(now.Add(delay))
		}
		c.emit(state)
		return delay
	}
	hash := enrollment.PhoneHomeConfigHash(c.cfg, c.hostname)
	if c.configHash == "" {
		c.configHash = hash
	}
	if hash != c.configHash {
		c.source.Demand(false)
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
	if identityErr == nil && !reusable && c.cfg.GetString(par.URN) != "" && c.cfg.GetString(par.PrivateKey) != "" {
		identity = &enrollment.PersistedIdentity{URN: c.cfg.GetString(par.URN), PrivateKey: c.cfg.GetString(par.PrivateKey)}
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
		c.source.Demand(false)
		c.observation.NextCheck = time.Time{}
		return set("enrolled", "identity_confirmed_not_action_readiness", 0)
	}

	if a == nil && !known && (!c.cfg.GetBool(par.SelfEnroll) || !c.cfg.GetBool(par.APIKeyOnlyEnrollment)) {
		return set("blocked", "fresh_poc_requires_api_key_self_enrollment", slowRetry)
	}
	// Demand cannot force discovery or shorten either owner's deadlines.
	c.source.Demand(!reusable && c.observation.RCReady)
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
			if outcome.Category == "missing_scope" || outcome.Category == "invalid_credentials" {
				// Persist the revalidation floor; rereading a cached grant is not a new check.
				a.RevalidateAfter = now
			}
			if err := enrollment.SavePhoneHomeSchedule(c.cfg, a); err != nil {
				return set("blocked", "cooldown_persistence_failed", slowRetry)
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
			if !c.observation.RCReady {
				return set("waiting", c.observation.RCReason, pollInterval)
			}
			a.Recovering, a.StartedAt = true, now
			a.RecoveryCount++
			a.NextAttemptAt = time.Time{} // next failure gets its own durable cooldown
			if err := enrollment.SavePhoneHomeSchedule(c.cfg, a); err != nil {
				return set("blocked", "recovery_schedule_failed", slowRetry)
			}
			return c.launch(ctx, now, control.Detail.Uuid, false, time.Time{}, set)
		}
		// Rejections require current RC readiness and a new enough PAR observation.
		if !c.observation.RCReady {
			return set("waiting", c.observation.RCReason, pollInterval)
		}
		if !a.RevalidateAfter.IsZero() && !c.observation.ScopeCheckedAt.After(a.RevalidateAfter) {
			return set("waiting", "par_scope_revalidation_required", pollInterval)
		}
	}
	if active(control.Detail) || active(executor.Detail) {
		return set("blocked", "adopting_existing_process", time.Minute)
	}
	if !c.observation.RCReady {
		return set("waiting", c.observation.RCReason, pollInterval)
	}
	if !reusable && !c.observation.ScopeReady {
		return set("waiting", c.observation.ScopeReason, pollInterval)
	}
	if ctx.Err() != nil || !c.cfg.GetBool(par.Enabled) {
		c.source.Demand(false)
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
	floor := time.Time{}
	if a != nil {
		floor = a.RevalidateAfter
	}
	return c.launch(ctx, now, control.Detail.Uuid, !reusable, floor, set)
}

func (c *controller) launch(ctx context.Context, now time.Time, uuid string, needsScope bool, floor time.Time, set func(string, string, time.Duration) time.Duration) time.Duration {
	abort := func(reason string, delay time.Duration) time.Duration {
		// No Start RPC was sent. Preserve this proven no-submission outcome so
		// a reservation cannot become permanently ambiguous when the gate closes.
		if needsScope {
			a, err := enrollment.ReadPhoneHomeAttempt(c.cfg)
			if err != nil || a == nil {
				return set("blocked", "unstarted_attempt_unreadable", slowRetry)
			}
			if err := enrollment.RecordPhoneHomeOutcome(c.cfg, enrollment.PhoneHomeOutcome{AttemptID: a.ID, EnrollmentFailure: opms.EnrollmentFailure{Category: "not_submitted", RetrySafe: true}}); err != nil {
				return set("blocked", "unstarted_outcome_not_saved", slowRetry)
			}
		}
		return set("waiting", reason, delay)
	}
	if ctx.Err() != nil || !c.cfg.GetBool(par.Enabled) {
		c.source.Demand(false)
		return abort("disabled_or_stopping", 0)
	}
	c.observation = c.source.Read(c.now())
	if !c.observation.RCReady {
		c.source.Demand(false)
		return abort(c.observation.RCReason, pollInterval)
	}
	if needsScope && (!c.observation.ScopeReady || (!floor.IsZero() && !c.observation.ScopeCheckedAt.After(floor))) {
		return abort("par_scope_revalidation_required", pollInterval)
	}
	c.started = now
	startCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := c.pm.Start(startCtx, &pb.StartRequest{NameOrUuid: uuid}); err != nil {
		return set("blocked", "launch_outcome_unknown", time.Minute)
	}
	return set("launching", "awaiting_persisted_identity", time.Second)
}

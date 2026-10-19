// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsprofile

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"maps"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/profilerec"
)

const (
	cfgEnabled           = "health_platform.logs_profile_recommendation.enabled"
	cfgInterval          = "health_platform.logs_profile_recommendation.interval"
	cfgMinHealthyPeriod  = "health_platform.logs_profile_recommendation.min_healthy_period"
	cfgMinSaturated30m   = "health_platform.logs_profile_recommendation.efficiency_min_saturated_30m"
	cfgForwarderInterval = "health_platform.forwarder.interval"
)

// maxListed caps each settings list on the wire.
const maxListed = 10

type held struct {
	kind    kind
	context map[string]string
}

type checker struct {
	cfg      config.Component
	hostname hostnameinterface.Component
	window   profilerec.LossWindow
	started  time.Time

	logsRunning   func() bool
	backpressure  func() logsmetrics.BackpressureSummary
	counters      func() profilerec.Counters
	activeProfile func() string
	plan          func(candidate string) (profilerec.Plan, bool)
	now           func() time.Time

	mu           sync.Mutex
	held         *held
	healthySince time.Time
}

func newChecker(cfg config.Component, hostname hostnameinterface.Component) *checker {
	c := &checker{
		cfg:           cfg,
		hostname:      hostname,
		logsRunning:   logsmetrics.LogsAgentRunning,
		backpressure:  logsmetrics.BackpressureSnapshot,
		counters:      func() profilerec.Counters { return profilerec.ReadCounters(logsmetrics.LogsExpvars) },
		activeProfile: func() string { return profilerec.ActiveProfile(cfg) },
		plan:          func(candidate string) (profilerec.Plan, bool) { return profilerec.PlanProfile(cfg, candidate) },
		now:           time.Now,
	}
	c.started = c.now()
	return c
}

// logsEnabled mirrors the logs agent's own gate, which still honours the deprecated log_enabled.
func logsEnabled(cfg config.Component) bool {
	return cfg.GetBool("logs_enabled") || cfg.GetBool("log_enabled")
}

type observation struct {
	stages        []profilerec.Stage
	state         string
	counters      profilerec.Counters
	dropped       bool
	missed        bool
	delivering    bool
	activeProfile string
}

func (o observation) healthy() bool {
	return !o.missed && o.state == logsmetrics.BackpressureHealthy
}

// Run reports the profile to apply and keeps reporting it until the pipeline has stayed
// healthy for min_healthy_period.
func (c *checker) Run() ([]runnerdef.IssueReport, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.cfg.GetBool(cfgEnabled) || !logsEnabled(c.cfg) {
		c.held, c.healthySince = nil, time.Time{}
		return nil, nil
	}
	if !c.logsRunning() {
		return nil, fmt.Errorf("logsprofile: logs agent not running: %w", runnerdef.ErrStateUnknown)
	}
	summary := c.backpressure()
	if summary.State == "" {
		return nil, fmt.Errorf("logsprofile: no pipeline monitor registered: %w", runnerdef.ErrStateUnknown)
	}

	now := c.now()
	counters := c.counters()
	dropped, missed, delivering := c.window.Observe(counters, now)
	obs := observation{
		stages:        profilerec.StagesFromBackpressure(summary.Components),
		state:         summary.State,
		counters:      counters,
		dropped:       dropped,
		missed:        missed,
		delivering:    delivering,
		activeProfile: c.activeProfile(),
	}

	if next := c.condition(obs); next != nil {
		c.held, c.healthySince = next, time.Time{}
		return c.report(next), nil
	}

	if !obs.healthy() {
		c.healthySince = time.Time{}
	} else if c.healthySince.IsZero() {
		c.healthySince = now
	}
	settled := !c.healthySince.IsZero() && now.Sub(c.healthySince) >= c.cfg.GetDuration(cfgMinHealthyPeriod)

	if c.held == nil {
		// A persisted issue from before a restart stays open until the pipeline settles, or until the verify
		// window passes with nothing left to recommend (e.g. the profile is applied but loss continues).
		if settled || now.Sub(c.started) >= c.verifyWindow() {
			return nil, nil
		}
		return nil, fmt.Errorf("logsprofile: settling: %w", runnerdef.ErrStateUnknown)
	}
	if settled {
		c.held = nil
		return nil, nil
	}
	return c.report(c.held), nil
}

// condition returns the issue that holds right now, or nil.
func (c *checker) condition(obs observation) *held {
	rec := profilerec.Recommend(obs.stages, obs.activeProfile, profilerec.Signals{
		MissedRecently:  obs.missed,
		Delivering:      obs.delivering,
		SenderLatencyMs: obs.counters.SenderLatencyMs,
	})
	if rec != nil {
		return c.build(recommended, rec.Profile, rec.ReasonCode, rec.Reason, rec.Bottleneck, obs)
	}

	holdingHigh := c.held != nil && c.held.kind == recommended
	if obs.missed || !obs.delivering || obs.state == logsmetrics.BackpressureHealthy || holdingHigh {
		return nil
	}
	bottleneck := profilerec.Bottleneck(obs.stages)
	if bottleneck == "" {
		return nil
	}
	saturated := time.Duration(saturated30mSeconds(obs.stages, bottleneck)) * time.Second
	if saturated < c.cfg.GetDuration(cfgMinSaturated30m) {
		return nil
	}
	profile, code, reason := profilerec.ForBottleneck(bottleneck, obs.counters.SenderLatencyMs)
	if profile == obs.activeProfile || profilerec.Covers(obs.activeProfile, profile) {
		return nil
	}
	return c.build(suggested, profile, code, reason, bottleneck, obs)
}

func saturated30mSeconds(stages []profilerec.Stage, name string) int64 {
	var longest int64
	for _, s := range stages {
		if s.Name == name {
			longest = max(longest, s.Saturated30mSeconds)
		}
	}
	return longest
}

// build returns nil when the profile would change nothing on this host.
func (c *checker) build(k kind, profile, code, reason, bottleneck string, obs observation) *held {
	plan, ok := c.plan(profile)
	if !ok || len(plan.Changes) == 0 {
		return nil
	}

	w := recommendation{
		Profile:             plan.Name,
		ProfileVersion:      plan.Version,
		ProfileDescription:  plan.Description,
		ProfileKeySource:    plan.ProfileKeySource,
		ReasonCode:          code,
		Reason:              reason,
		Bottleneck:          bottleneck,
		BackpressureState:   obs.state,
		SenderLatencyMs:     obs.counters.SenderLatencyMs,
		DroppedRecently:     obs.dropped,
		MissedRecently:      obs.missed,
		Saturated30mSeconds: saturated30mSeconds(obs.stages, bottleneck),
		ActiveProfile:       obs.activeProfile,
		VerifyWindowSeconds: int64(c.verifyWindow().Seconds()),
	}
	for _, s := range plan.Current[:min(len(plan.Current), maxListed)] {
		w.Current = append(w.Current, settingWire{Key: s.Key, Value: scalar(s.Value), Source: s.Source})
	}
	for _, ch := range plan.Changes[:min(len(plan.Changes), maxListed)] {
		w.Changes = append(w.Changes, changeWire{Key: ch.Key, From: scalar(ch.From), To: scalar(ch.To)})
	}
	for _, b := range plan.Blocked[:min(len(plan.Blocked), maxListed)] {
		w.Blocked = append(w.Blocked, blockedWire{Key: b.Key, Source: b.Source})
	}

	encoded, err := json.Marshal(w)
	if err != nil {
		return nil
	}
	return &held{kind: k, context: map[string]string{contextKeyRecommendation: string(encoded)}}
}

// verifyWindow is how long after a deploy the issue should take to clear: settle, then reach the backend.
func (c *checker) verifyWindow() time.Duration {
	return c.cfg.GetDuration(cfgMinHealthyPeriod) + c.cfg.GetDuration(cfgInterval) + c.cfg.GetDuration(cfgForwarderInterval)
}

func (c *checker) report(h *held) []runnerdef.IssueReport {
	return []runnerdef.IssueReport{{
		IssueID:   hostIssueID(h.kind.idPrefix, c.hostname.GetSafe(context.Background())),
		IssueName: h.kind.name,
		Source:    issueSource,
		Context:   maps.Clone(h.context),
	}}
}

// hostIssueID scopes the id to this host: the backend dedups on id alone.
func hostIssueID(prefix, hostname string) string {
	h := fnv.New64a()
	h.Write([]byte(hostname)) // never returns an error for hash.Hash
	return fmt.Sprintf("%s:%016x", prefix, h.Sum64())
}

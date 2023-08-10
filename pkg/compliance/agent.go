// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compliance TODO comment
package compliance

import (
	"context"
	"expvar"
	"fmt"
	"hash/fnv"
	"math/rand"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance/aptconfig"
	"github.com/DataDog/datadog-agent/pkg/compliance/k8sconfig"
	"github.com/DataDog/datadog-agent/pkg/compliance/metrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	secl "github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const containersCountMetricName = "datadog.security_agent.compliance.containers_running"

var status = expvar.NewMap("compliance")

// AgentOptions exported type should have comment or be unexported
type AgentOptions struct {
	// ResolverOptions is the options passed to the constructed resolvers
	// internally. See resolver.go.
	ResolverOptions

	// ConfigDir is the directory in which benchmarks files and assets are
	// defined.
	ConfigDir string

	// Reporter is the output interface of the events that are gathered by the
	// agent.
	Reporter *LogReporter

	// RuleFilter allow specifying a global rule filtering that will be
	// applied on all loaded benchmarks.
	RuleFilter RuleFilter

	// CheckInterval is the period at which benchmarks are being run. It should
	// also be roughly (see RunJitterMax) the interval at which rule checks
	// are being run. By default: 20 minutes.
	CheckInterval time.Duration

	// RunJitterMax is the maximum duration of the jitter that randomizes the
	// benchmarks evaluations. If less than 0, the jitter is null. By default
	// is is one tenth of the specified CheckInterval.
	RunJitterMax time.Duration

	// EvalThrottling is the time that space out rule evaluation to avoid CPU
	// spikes.
	EvalThrottling time.Duration
}

// Agent exported type should have comment or be unexported
type Agent struct {
	opts AgentOptions

	telemetry  *common.ContainersTelemetry
	statuses   map[string]*CheckStatus
	statusesMu sync.RWMutex

	finish chan struct{}
	cancel context.CancelFunc
}

func xccdfEnabled() bool {
	return config.Datadog.GetBool("compliance_config.xccdf.enabled") || config.Datadog.GetBool("compliance_config.host_benchmarks.enabled")
}

// DefaultRuleFilter exported function should have comment or be unexported
func DefaultRuleFilter(r *Rule) bool {
	if config.IsKubernetes() {
		if r.SkipOnK8s {
			return false
		}
	} else {
		if r.HasScope(KubernetesNodeScope) || r.HasScope(KubernetesClusterScope) {
			return false
		}
	}
	if r.IsXCCDF() && !xccdfEnabled() {
		return false
	}
	if len(r.Filters) > 0 {
		ruleFilterModel := rules.NewRuleFilterModel()
		seclRuleFilter := secl.NewSECLRuleFilter(ruleFilterModel)
		accepted, err := seclRuleFilter.IsRuleAccepted(&secl.RuleDefinition{
			Filters: r.Filters,
		})
		if err != nil {
			log.Errorf("failed to apply rule filters: %s", err)
			return false
		}
		if !accepted {
			return false
		}
	}
	return true
}

// NewAgent exported function should have comment or be unexported
func NewAgent(opts AgentOptions) *Agent {
	if opts.ConfigDir == "" {
		panic("compliance: missing agent configuration directory")
	}
	if opts.Reporter == nil {
		panic("compliance: missing agent reporter")
	}
	if opts.Reporter.Endpoints() == nil {
		panic("compliance: missing agent endpoints")
	}
	if opts.CheckInterval == 0 {
		opts.CheckInterval = 20 * time.Minute
	}
	if opts.RunJitterMax == 0 {
		opts.RunJitterMax = opts.CheckInterval / 10
	} else if opts.RunJitterMax < 0 {
		opts.RunJitterMax = 0
	}
	if opts.EvalThrottling == 0 {
		opts.EvalThrottling = 2 * time.Second
	}
	if ruleFilter := opts.RuleFilter; ruleFilter != nil {
		opts.RuleFilter = func(r *Rule) bool { return DefaultRuleFilter(r) && ruleFilter(r) }
	} else {
		opts.RuleFilter = func(r *Rule) bool { return DefaultRuleFilter(r) }
	}
	return &Agent{
		opts:     opts,
		statuses: make(map[string]*CheckStatus),
	}
}

// Start exported method should have comment or be unexported
func (a *Agent) Start() error {
	telemetry, err := common.NewContainersTelemetry()
	if err != nil {
		log.Errorf("could not start containers telemetry: %v", err)
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.telemetry = telemetry
	a.cancel = cancel
	a.finish = make(chan struct{})

	status.Set(
		"Checks",
		expvar.Func(func() interface{} {
			return a.getChecksStatus()
		}),
	)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		a.runTelemetry(ctx)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		a.runRegoBenchmarks(ctx)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		a.runXCCDFBenchmarks(ctx)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		a.runKubernetesConfigurationsExport(ctx)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		a.runAptConfigurationExport(ctx)
		wg.Done()
	}()

	go func() {
		<-ctx.Done()
		wg.Wait()
		close(a.finish)
	}()

	log.Infof("compliance agent started")
	return nil
}

// Stop exported method should have comment or be unexported
func (a *Agent) Stop() {
	log.Tracef("shutting down compliance agent")
	a.cancel()
	select {
	case <-time.After(10 * time.Second):
	case <-a.finish:
	}
	log.Infof("compliance agent shut down")
}

func (a *Agent) runRegoBenchmarks(ctx context.Context) {
	benchmarks, err := LoadBenchmarks(a.opts.ConfigDir, "*.yaml", func(r *Rule) bool {
		return r.IsRego() && a.opts.RuleFilter(r)
	})
	if err != nil {
		log.Warnf("could not load rego benchmarks: %v", err)
		return
	}
	if len(benchmarks) == 0 {
		log.Infof("no rego benchmark to run")
		return
	}
	a.addBenchmarks(benchmarks...)

	runTicker := time.NewTicker(a.opts.CheckInterval)
	throttler := time.NewTicker(a.opts.EvalThrottling)
	defer runTicker.Stop()
	defer throttler.Stop()

	log.Debugf("will be executing %d rego benchmarks every %s", len(benchmarks), a.opts.CheckInterval)
	for {
		for i, benchmark := range benchmarks {
			seed := fmt.Sprintf("%s%s%d", a.opts.Hostname, benchmark.FrameworkID, i)
			if sleepAborted(ctx, time.After(randomJitter(seed, a.opts.RunJitterMax))) {
				return
			}
			resolver := NewResolver(ctx, a.opts.ResolverOptions)
			for _, rule := range benchmark.Rules {
				events := ResolveAndEvaluateRegoRule(ctx, resolver, benchmark, rule)
				a.reportEvents(ctx, benchmark, events)
				if sleepAborted(ctx, throttler.C) {
					resolver.Close()
					return
				}
			}
			resolver.Close()
		}
		if sleepAborted(ctx, runTicker.C) {
			return
		}
	}
}

func (a *Agent) runXCCDFBenchmarks(ctx context.Context) {
	if !xccdfEnabled() {
		return
	}
	benchmarks, err := LoadBenchmarks(a.opts.ConfigDir, "*.yaml", func(r *Rule) bool {
		return r.IsXCCDF() && a.opts.RuleFilter(r)
	})
	if err != nil {
		log.Warnf("could not load xccdf benchmarks: %v", err)
		return
	}
	if len(benchmarks) == 0 {
		log.Infof("no xccdf benchmark to run")
		return
	}
	a.addBenchmarks(benchmarks...)

	runTicker := time.NewTicker(a.opts.CheckInterval)
	throttler := time.NewTicker(a.opts.EvalThrottling)
	defer runTicker.Stop()
	defer throttler.Stop()

	log.Debugf("will be executing %d XCCDF benchmarks every %s", len(benchmarks), a.opts.CheckInterval)
	for {
		for i, benchmark := range benchmarks {
			seed := fmt.Sprintf("%s%s%d", a.opts.Hostname, benchmark.FrameworkID, i)
			if sleepAborted(ctx, time.After(randomJitter(seed, a.opts.RunJitterMax))) {
				return
			}
			for _, rule := range benchmark.Rules {
				events := EvaluateXCCDFRule(ctx, a.opts.Hostname, a.opts.StatsdClient, benchmark, rule)
				a.reportEvents(ctx, benchmark, events)
				if sleepAborted(ctx, throttler.C) {
					return
				}
			}
		}
		if sleepAborted(ctx, runTicker.C) {
			return
		}
	}
}

func (a *Agent) runKubernetesConfigurationsExport(ctx context.Context) {
	if !config.IsKubernetes() {
		return
	}

	runTicker := time.NewTicker(a.opts.CheckInterval)
	defer runTicker.Stop()

	for i := 0; ; i++ {
		seed := fmt.Sprintf("%s%s%d", a.opts.Hostname, "kubernetes-configuration", i)
		jitter := randomJitter(seed, a.opts.RunJitterMax)
		if sleepAborted(ctx, time.After(jitter)) {
			return
		}
		k8sResourceType, k8sResourceData := k8sconfig.LoadConfiguration(ctx, a.opts.HostRoot)
		k8sResourceLog := NewResourceLog(a.opts.Hostname, k8sResourceType, k8sResourceData)
		a.opts.Reporter.ReportEvent(k8sResourceLog)
		if sleepAborted(ctx, runTicker.C) {
			return
		}
	}
}

func (a *Agent) runAptConfigurationExport(ctx context.Context) {
	ruleFilterModel := rules.NewRuleFilterModel()
	seclRuleFilter := secl.NewSECLRuleFilter(ruleFilterModel)
	accepted, err := seclRuleFilter.IsRuleAccepted(&secl.RuleDefinition{
		Filters: []string{aptconfig.SeclFilter},
	})
	if !accepted || err != nil {
		return
	}

	runTicker := time.NewTicker(a.opts.CheckInterval)
	defer runTicker.Stop()

	for i := 0; ; i++ {
		seed := fmt.Sprintf("%s%s%d", a.opts.Hostname, "apt-configuration", i)
		jitter := randomJitter(seed, a.opts.RunJitterMax)
		if sleepAborted(ctx, time.After(jitter)) {
			return
		}
		aptResourceType, aptResourceData := aptconfig.LoadConfiguration(ctx, a.opts.HostRoot)
		aptResourceLog := NewResourceLog(a.opts.Hostname, aptResourceType, aptResourceData)
		a.opts.Reporter.ReportEvent(aptResourceLog)
		if sleepAborted(ctx, runTicker.C) {
			return
		}
	}
}

func (a *Agent) reportEvents(ctx context.Context, benchmark *Benchmark, events []*CheckEvent) {
	for _, event := range events {
		a.updateEvent(event)
		if event.Result == CheckSkipped {
			continue
		}
		a.opts.Reporter.ReportEvent(event)
	}
}

func (a *Agent) runTelemetry(ctx context.Context) {
	log.Info("Start collecting Compliance telemetry")
	defer log.Info("Stopping Compliance telemetry")

	metricsTicker := time.NewTicker(1 * time.Minute)
	defer metricsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-metricsTicker.C:
			a.telemetry.ReportContainers(containersCountMetricName)
		}
	}
}

// GetStatus exported method should have comment or be unexported
func (a *Agent) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"endpoints": a.opts.Reporter.Endpoints().GetStatus(),
	}
}

func (a *Agent) getChecksStatus() interface{} {
	a.statusesMu.RLock()
	defer a.statusesMu.RUnlock()
	statuses := make([]*CheckStatus, 0, len(a.statuses))
	for _, status := range a.statuses {
		statuses = append(statuses, status)
	}
	return statuses
}

func (a *Agent) addBenchmarks(benchmarks ...*Benchmark) {
	a.statusesMu.Lock()
	defer a.statusesMu.Unlock()
	for _, benchmark := range benchmarks {
		for _, rule := range benchmark.Rules {
			if _, ok := a.statuses[rule.ID]; ok {
				continue
			}
			a.statuses[rule.ID] = &CheckStatus{
				RuleID:      rule.ID,
				Description: rule.Description,
				Name:        fmt.Sprintf("%s: %s", rule.ID, rule.Description),
				Framework:   benchmark.FrameworkID,
				Source:      benchmark.Source,
				Version:     benchmark.Version,
				InitError:   nil,
			}
		}
	}
}

func (a *Agent) updateEvent(event *CheckEvent) {
	if client := a.opts.StatsdClient; client != nil {
		tags := []string{
			"rule_id:" + event.RuleID,
			"rule_result:" + string(event.Result),
			"agent_version:" + event.AgentVersion,
		}
		if err := client.Gauge(metrics.MetricChecksStatuses, 1, tags, 1.0); err != nil {
			log.Errorf("failed to send checks metric: %v", err)
		}
	}

	a.statusesMu.Lock()
	defer a.statusesMu.Unlock()
	status, ok := a.statuses[event.RuleID]
	if !ok || status == nil {
		log.Errorf("check for rule=%s was not registered in checks monitor statuses", event.RuleID)
	} else {
		status.LastEvent = event
	}
}

func sleepAborted(ctx context.Context, ch <-chan time.Time) bool {
	select {
	case <-ch:
		return false
	case <-ctx.Done():
		return true
	}
}

func randomJitter(seed string, maxDuration time.Duration) time.Duration {
	if maxDuration == 0 {
		return 0
	}
	h := fnv.New64a()
	h.Write([]byte(seed))
	r := rand.New(rand.NewSource(int64(h.Sum64())))
	d := r.Int63n(maxDuration.Milliseconds())
	return time.Duration(d) * time.Millisecond
}

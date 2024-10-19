// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compliance implements a specific part of the datadog-agent
// responsible for scanning host and containers and report various
// misconfigurations and compliance issues.
package compliance

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"expvar"
	"fmt"
	"hash/fnv"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/compliance/aptconfig"
	"github.com/DataDog/datadog-agent/pkg/compliance/dbconfig"
	"github.com/DataDog/datadog-agent/pkg/compliance/k8sconfig"
	"github.com/DataDog/datadog-agent/pkg/compliance/metrics"
	"github.com/DataDog/datadog-agent/pkg/compliance/utils"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	secl "github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const containersCountMetricName = "datadog.security_agent.compliance.containers_running"

var status = expvar.NewMap("compliance")

const (
	// defaultEvalThrottling is the time that space out rule evaluation to avoid CPU
	// spikes.
	defaultEvalThrottling = 2 * time.Second

	// defaultCheckInterval defines the default value used as interval for
	// executing benchmarks.
	defaultCheckInterval = 20 * time.Minute

	// defaultCheckInterval defines the default value used as interval for
	// executing benchmarks with a low priority: usually because of the
	// compute overhead of executing such benchmarks, or the nature of
	// configurations which tend to be constant.
	defaultCheckIntervalLowPriority = 3 * time.Hour
)

// AgentOptions holds the different options to configure the compliance agent.
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

	// CheckInterval is the period at which benchmarks are being run. It
	// should also be roughly the interval at which rule checks are being run.
	// By default: 20 minutes.
	CheckInterval time.Duration

	// CheckIntervalLowPriority is like CheckInterval but for low-priority
	// benchmarks.
	CheckIntervalLowPriority time.Duration

	// EnabledConfigurationExporters lists configuration exporter that shall be
	// enabled.
	EnabledConfigurationExporters []ConfigurationExporter

	// SysProbeClient is the HTTP client to allow the execution of benchmarks
	// from system-probe. see: cmd/system-probe/modules/compliance.go
	SysProbeClient *http.Client
}

// ConfigurationExporter is an enum type defining all configuration export
// configuration processes.
type ConfigurationExporter int

const (
	// KubernetesExporter exports Kubernetes components configuration running
	// on the system.
	KubernetesExporter ConfigurationExporter = iota

	// AptExporter exports local APT configuration data.
	AptExporter

	// DBExporter exports local or containerized DB application configuration
	// data.
	DBExporter
)

// Agent is the compliance agent that is responsible for running compliance
// continuously benchmarks and configuration checking.
type Agent struct {
	telemetrySender telemetry.SimpleTelemetrySender
	wmeta           workloadmeta.Component
	opts            AgentOptions

	telemetry  *telemetry.ContainersTelemetry
	statuses   map[string]*CheckStatus
	statusesMu sync.RWMutex

	finish chan struct{}
	cancel context.CancelFunc

	k8sManaged *string
}

func xccdfEnabled() bool {
	return pkgconfigsetup.Datadog().GetBool("compliance_config.xccdf.enabled") || pkgconfigsetup.Datadog().GetBool("compliance_config.host_benchmarks.enabled")
}

var defaultSECLRuleFilter = sync.OnceValues(func() (*secl.SECLRuleFilter, error) {
	ruleFilterModel, err := rules.NewRuleFilterModel(nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create default SECL rule filter: %w", err)
	}
	filter := secl.NewSECLRuleFilter(ruleFilterModel)
	return filter, nil
})

// DefaultRuleFilter implements the default filtering of benchmarks' rules. It
// will exclude rules based on the evaluation context / environment running
// the benchmark.
func DefaultRuleFilter(r *Rule) bool {
	if env.IsKubernetes() {
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
		seclRuleFilter, err := defaultSECLRuleFilter()
		if err != nil {
			log.Errorf("failed to apply rule filters: %s", err)
			return false
		}

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

// NewAgent returns a new compliance agent.
func NewAgent(telemetrySender telemetry.SimpleTelemetrySender, wmeta workloadmeta.Component, opts AgentOptions) *Agent {
	if opts.ConfigDir == "" {
		panic("compliance: missing agent configuration directory")
	}
	if opts.Reporter == nil {
		panic("compliance: missing agent reporter")
	}
	if opts.Reporter.Endpoints() == nil {
		panic("compliance: missing agent endpoints")
	}
	if opts.CheckInterval <= 0 {
		opts.CheckInterval = defaultCheckInterval
	}
	if opts.CheckIntervalLowPriority <= 0 {
		opts.CheckIntervalLowPriority = defaultCheckIntervalLowPriority
	}
	if ruleFilter := opts.RuleFilter; ruleFilter != nil {
		opts.RuleFilter = func(r *Rule) bool { return DefaultRuleFilter(r) && ruleFilter(r) }
	} else {
		opts.RuleFilter = func(r *Rule) bool { return DefaultRuleFilter(r) }
	}
	return &Agent{
		telemetrySender: telemetrySender,
		wmeta:           wmeta,
		opts:            opts,
		statuses:        make(map[string]*CheckStatus),
	}
}

// Start starts the compliance agent.
func (a *Agent) Start() error {
	telemetry, err := telemetry.NewContainersTelemetry(a.telemetrySender, a.wmeta)
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

	_, k8sResourceData := k8sconfig.LoadConfiguration(ctx, a.opts.HostRoot)
	if k8sResourceData != nil && k8sResourceData.ManagedEnvironment != nil {
		a.k8sManaged = &k8sResourceData.ManagedEnvironment.Name
	}

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

	for _, conf := range a.opts.EnabledConfigurationExporters {
		switch conf {
		case AptExporter:
			wg.Add(1)
			go func() {
				a.runAptConfigurationExport(ctx)
				wg.Done()
			}()

		case KubernetesExporter:
			wg.Add(1)
			go func() {
				a.runKubernetesConfigurationsExport(ctx)
				wg.Done()
			}()

		case DBExporter:
			wg.Add(1)
			go func() {
				a.runDBConfigurationsExport(ctx)
				wg.Done()
			}()
		}
	}

	go func() {
		<-ctx.Done()
		wg.Wait()
		close(a.finish)
	}()

	log.Infof("compliance agent started")
	return nil
}

// Stop stops the compliance agent.
func (a *Agent) Stop() {
	log.Tracef("shutting down compliance agent")
	a.cancel()
	select {
	case <-time.After(20 * time.Second):
	case <-a.finish:
	}
	a.opts.Reporter.Stop()
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

	checkInterval := a.opts.CheckInterval
	runTicker := time.NewTicker(checkInterval)
	throttler := time.NewTicker(defaultEvalThrottling)
	defer runTicker.Stop()
	defer throttler.Stop()

	log.Debugf("will be executing %d rego benchmarks every %s", len(benchmarks), checkInterval)
	for runCount := uint64(0); ; runCount++ {
		for _, benchmark := range benchmarks {
			if sleepRandomJitter(ctx, checkInterval, runCount, a.opts.Hostname, benchmark.FrameworkID) {
				return
			}

			resolver := NewResolver(ctx, a.opts.ResolverOptions)
			for _, rule := range benchmark.Rules {
				inputs, err := resolver.ResolveInputs(ctx, rule)
				if err := ctx.Err(); err != nil {
					return
				}
				if err != nil {
					a.reportCheckEvents(checkInterval, CheckEventFromError(RegoEvaluator, rule, benchmark, err))
				} else {
					events := EvaluateRegoRule(ctx, inputs, benchmark, rule)
					a.reportCheckEvents(checkInterval, events...)
				}

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

	checkInterval := a.opts.CheckIntervalLowPriority
	runTicker := time.NewTicker(checkInterval)
	throttler := time.NewTicker(defaultEvalThrottling)
	defer runTicker.Stop()
	defer throttler.Stop()

	log.Debugf("will be executing %d XCCDF benchmarks every %s", len(benchmarks), checkInterval)
	for runCount := uint64(0); ; runCount++ {
		for _, benchmark := range benchmarks {
			if sleepRandomJitter(ctx, checkInterval, runCount, a.opts.Hostname, benchmark.FrameworkID) {
				FinishXCCDFBenchmark(ctx, benchmark)
				return
			}
			for _, rule := range benchmark.Rules {
				events := EvaluateXCCDFRule(ctx, a.opts.Hostname, a.opts.StatsdClient, benchmark, rule)
				a.reportCheckEvents(checkInterval, events...)
				if sleepAborted(ctx, throttler.C) {
					FinishXCCDFBenchmark(ctx, benchmark)
					return
				}
			}
			FinishXCCDFBenchmark(ctx, benchmark)
		}
		if sleepAborted(ctx, runTicker.C) {
			return
		}
	}
}

func (a *Agent) runKubernetesConfigurationsExport(ctx context.Context) {
	if !env.IsKubernetes() {
		return
	}

	checkInterval := a.opts.CheckIntervalLowPriority
	runTicker := time.NewTicker(checkInterval)
	defer runTicker.Stop()

	for runCount := uint64(0); ; runCount++ {
		if sleepRandomJitter(ctx, checkInterval, runCount, a.opts.Hostname, "kubernetes-configuration") {
			return
		}
		k8sResourceType, k8sResourceData := k8sconfig.LoadConfiguration(ctx, a.opts.HostRoot)
		a.reportResourceLog(checkInterval, NewResourceLog(a.opts.Hostname, k8sResourceType, k8sResourceData))
		if sleepAborted(ctx, runTicker.C) {
			return
		}
	}
}

func (a *Agent) runAptConfigurationExport(ctx context.Context) {
	ruleFilterModel, err := rules.NewRuleFilterModel(nil, "")
	if err != nil {
		log.Errorf("failed to run apt configuration export: %v", err)
		return
	}

	seclRuleFilter := secl.NewSECLRuleFilter(ruleFilterModel)
	accepted, err := seclRuleFilter.IsRuleAccepted(&secl.RuleDefinition{
		Filters: []string{aptconfig.SeclFilter},
	})
	if !accepted || err != nil {
		return
	}

	checkInterval := a.opts.CheckIntervalLowPriority
	runTicker := time.NewTicker(checkInterval)
	defer runTicker.Stop()

	for runCount := uint64(0); ; runCount++ {
		if sleepRandomJitter(ctx, checkInterval, runCount, a.opts.Hostname, "apt-configuration") {
			return
		}
		aptResourceType, aptResourceData := aptconfig.LoadConfiguration(ctx, a.opts.HostRoot)
		a.reportResourceLog(checkInterval, NewResourceLog(a.opts.Hostname, aptResourceType, aptResourceData))
		if sleepAborted(ctx, runTicker.C) {
			return
		}
	}
}

func (a *Agent) runDBConfigurationsExport(ctx context.Context) {
	checkInterval := a.opts.CheckIntervalLowPriority
	runTicker := time.NewTicker(checkInterval)
	defer runTicker.Stop()

	for runCount := uint64(0); ; runCount++ {
		if sleepRandomJitter(ctx, checkInterval, runCount, a.opts.Hostname, "db-configuration") {
			return
		}
		procs, err := process.ProcessesWithContext(ctx)
		if err != nil {
			continue
		}
		groups := groupProcesses(procs, dbconfig.GetProcResourceType)
		for keyGroup, proc := range groups {
			if keyGroup.containerID != "" {
				if err := a.reportDBConfigurationFromSystemProbe(ctx, keyGroup.containerID, proc.Pid); err != nil {
					log.Warnf("error reporting DB configuration from system-probe: %s", err)
				}
			} else {
				resourceType, resource, ok := dbconfig.LoadConfiguration(ctx, a.opts.HostRoot, proc)
				if ok {
					log := NewResourceLog(a.opts.Hostname, resourceType, resource)
					a.reportResourceLog(defaultCheckIntervalLowPriority, log)
				}
			}
		}
		if sleepAborted(ctx, runTicker.C) {
			return
		}
	}
}

func (a *Agent) reportDBConfigurationFromSystemProbe(ctx context.Context, containerID utils.ContainerID, pid int32) error {
	if a.opts.SysProbeClient == nil {
		return fmt.Errorf("system-probe socket client was not created")
	}

	qs := make(url.Values)
	qs.Add("pid", strconv.FormatInt(int64(pid), 10))
	sysProbeComplianceModuleURL := &url.URL{
		Scheme:   "http",
		Host:     "unix",
		Path:     "/compliance/dbconfig",
		RawQuery: qs.Encode(),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sysProbeComplianceModuleURL.String(), nil)
	if err != nil {
		return err
	}

	resp, err := a.opts.SysProbeClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error running cross-container benchmark: %s", resp.Status)
	}

	var resource *dbconfig.DBResource
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, &resource); err != nil {
		return err
	}
	if resource != nil {
		dbResourceLog := NewResourceLog(a.opts.Hostname+"_"+string(containerID), resource.Type, resource.Config)
		dbResourceLog.Container = &CheckContainerMeta{
			ContainerID: string(containerID),
		}
		a.reportResourceLog(defaultCheckIntervalLowPriority, dbResourceLog)
	}
	return nil
}

type procGroup struct {
	key         string
	containerID utils.ContainerID
}

func groupProcesses(procs []*process.Process, getKey func(*process.Process) (string, bool)) map[procGroup]*process.Process {
	groups := make(map[procGroup]*process.Process)
	for _, proc := range procs {
		key, ok := getKey(proc)
		if !ok {
			continue
		}
		// if the process does not run in any form of container, containerID
		// is the empty string "" and it can be run locally
		containerID, _ := utils.GetProcessContainerID(proc.Pid)
		// We dedupe our scans based on the resource type and the container
		// ID, assuming that we will scan the same configuration for each
		// containers running the process.
		groupKey := procGroup{
			key:         key,
			containerID: containerID,
		}
		if _, ok := groups[groupKey]; !ok {
			groups[groupKey] = proc
		}
	}
	return groups
}

func (a *Agent) reportResourceLog(resourceTTL time.Duration, resourceLog *ResourceLog) {
	expireAt := time.Now().Add(2 * resourceTTL).Truncate(1 * time.Second)
	resourceLog.ExpireAt = &expireAt
	if a.wmeta != nil && resourceLog.Container != nil {
		if ctnr, _ := a.wmeta.GetContainer(resourceLog.Container.ContainerID); ctnr != nil {
			resourceLog.Container.ImageID = ctnr.Image.ID
			resourceLog.Container.ImageName = ctnr.Image.Name
			resourceLog.Container.ImageTag = ctnr.Image.Tag
		}
	}
	a.opts.Reporter.ReportEvent(resourceLog)
}

func (a *Agent) reportCheckEvents(eventsTTL time.Duration, events ...*CheckEvent) {
	eventsExpireAt := time.Now().Add(2 * eventsTTL).Truncate(1 * time.Second)
	for _, event := range events {
		event.ExpireAt = &eventsExpireAt
		a.updateEvent(event)
		if event.Result == CheckSkipped {
			continue
		}
		if a.wmeta != nil && event.Container != nil {
			if ctnr, _ := a.wmeta.GetContainer(event.Container.ContainerID); ctnr != nil {
				event.Container.ImageID = ctnr.Image.ID
				event.Container.ImageName = ctnr.Image.Name
				event.Container.ImageTag = ctnr.Image.Tag
			}
		}
		event.K8SManaged = a.k8sManaged
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

// sleepRandomJitter returns a jitter duration adapted to space out randomly
// our benchmark runs given the current run count and the run interval. The
// random timing is generated with a seeded RNG using the list of optional
// seeding strings and the runCount value.
func sleepRandomJitter(ctx context.Context, runInterval time.Duration, runCount uint64, seeds ...string) bool {
	// guardrail in case runInterval is set to a negative or null value.
	if runInterval <= 0 {
		select {
		case <-ctx.Done():
			return true
		default:
			return false
		}
	}

	// If we are jittering the first run, we hardcode a reasonably small
	// amount of time to make sure we init the checks quickly for short-lived
	// hosts. Otherwise we use the tenth of the run interval.
	const defaultJitterMax = 20 * time.Minute

	jitterMax := runInterval / 10
	if runCount == 0 && jitterMax > defaultJitterMax {
		jitterMax = defaultJitterMax
	}

	var runCountBuf [8]byte
	binary.LittleEndian.PutUint64(runCountBuf[:], runCount)

	h := fnv.New64a()
	h.Write(runCountBuf[:])
	for _, seed := range seeds {
		h.Write([]byte(seed))
	}

	r := rand.New(rand.NewSource(int64(h.Sum64())))
	d := r.Int63n(jitterMax.Milliseconds())
	sleep := time.Duration(d) * time.Millisecond
	return sleepAborted(ctx, time.After(sleep))
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package modules

import (
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/windowsdriver/ddinjector"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// minInjectorQueryDelay is the minimum delay between queries to the injector driver.
	minInjectorQueryDelay = 5 * time.Second
)

func init() { registerModule(Injector) }

var _ module.Module = &injectorModule{}

// Injector Factory
var Injector = &module.Factory{
	Name:             config.InjectorModule,
	ConfigNamespaces: []string{},
	Fn: func(_ *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {
		log.Infof("Creating Windows Injector module")

		m := &injectorModule{
			telemetry:      deps.Telemetry,
			sysProbeConfig: deps.SysprobeConfig,
		}

		m.initializeMetrics()

		return m, nil
	},
}

type injectorModule struct {
	lastCheck      atomic.Int64
	collectMutex   sync.Mutex
	telemetry      telemetry.Component
	counters       ddinjector.InjectorCounters
	sysProbeConfig sysprobeconfig.Component
}

// Register registers the endpoint for this module
func (m *injectorModule) Register(_ *module.Router) error {
	if m.sysProbeConfig.GetBool("injector.enable_telemetry") {
		log.Info("Windows Injector telemetry enabled")
		m.telemetry.RegisterCollector(m)
	} else {
		log.Info("Windows Injector telemetry disabled")
	}

	return nil
}

// Close cleans up module resources
func (m *injectorModule) Close() {
	if m.sysProbeConfig.GetBool("injector.enable_telemetry") {
		m.telemetry.UnregisterCollector(m)
	}
}

// GetStats prints the injector stats as part of /debug/stats.
func (m *injectorModule) GetStats() map[string]interface{} {
	// Sanity check in case the metrics have not been initialized.
	if m.counters.ProcessesAddedToInjectionTracker == nil {
		return map[string]interface{}{}
	}

	// If last_check_timestamp is 0, the Collect callback has not yet been trigered.
	return map[string]interface{}{
		"last_check_timestamp":                     m.lastCheck.Load(),
		"processes_added_to_injection_tracker":     m.counters.ProcessesAddedToInjectionTracker.Get(),
		"processes_removed_from_injection_tracker": m.counters.ProcessesRemovedFromInjectionTracker.Get(),
		"processes_skipped_subsystem":              m.counters.ProcessesSkippedSubsystem.Get(),
		"processes_skipped_container":              m.counters.ProcessesSkippedContainer.Get(),
		"processes_skipped_protected":              m.counters.ProcessesSkippedProtected.Get(),
		"processes_skipped_system":                 m.counters.ProcessesSkippedSystem.Get(),
		"processes_skipped_excluded":               m.counters.ProcessesSkippedExcluded.Get(),
		"injection_attempts":                       m.counters.InjectionAttempts.Get(),
		"injection_attempt_failures":               m.counters.InjectionAttemptFailures.Get(),
		"injection_max_time_us":                    m.counters.InjectionMaxTimeUs.Get(),
		"injection_successes":                      m.counters.InjectionSuccesses.Get(),
		"injection_failures":                       m.counters.InjectionFailures.Get(),
		"pe_caching_failures":                      m.counters.PeCachingFailures.Get(),
		"import_directory_restoration_failures":    m.counters.ImportDirectoryRestorationFailures.Get(),
		"pe_memory_allocation_failures":            m.counters.PeMemoryAllocationFailures.Get(),
		"pe_injection_context_allocated":           m.counters.PeInjectionContextAllocated.Get(),
		"pe_injection_context_cleanedup":           m.counters.PeInjectionContextCleanedup.Get(),
	}
}

////////////////////////////////////////////////////
// RAR/prometheus/telemetry related implementations

func (m *injectorModule) initializeMetrics() {
	const subsystem = "injector"

	m.counters.ProcessesAddedToInjectionTracker = m.telemetry.NewSimpleGauge(
		subsystem, "processes_added_to_injection_tracker",
		"Number of processes added to injection tracker")

	m.counters.ProcessesRemovedFromInjectionTracker = m.telemetry.NewSimpleGauge(
		subsystem, "processes_removed_from_injection_tracker",
		"Number of processes removed from injection tracker")

	m.counters.ProcessesSkippedSubsystem = m.telemetry.NewSimpleGauge(
		subsystem, "processes_skipped_subsystem",
		"Number of skipped subsystem processes")

	m.counters.ProcessesSkippedContainer = m.telemetry.NewSimpleGauge(
		subsystem, "processes_skipped_container",
		"Number of skipped container processes")

	m.counters.ProcessesSkippedProtected = m.telemetry.NewSimpleGauge(
		subsystem, "processes_skipped_protected",
		"Number of skipped protected processes")

	m.counters.ProcessesSkippedSystem = m.telemetry.NewSimpleGauge(
		subsystem, "processes_skipped_system",
		"Number of skipped system processes")

	m.counters.ProcessesSkippedExcluded = m.telemetry.NewSimpleGauge(
		subsystem, "processes_skipped_excluded",
		"Number of skipped processes due to exclusion")

	m.counters.InjectionAttempts = m.telemetry.NewSimpleGauge(
		subsystem, "injection_attempts",
		"Number of injection attempts")

	m.counters.InjectionAttemptFailures = m.telemetry.NewSimpleGauge(
		subsystem, "injection_attempt_failures",
		"Number of injection attempt failures")

	m.counters.InjectionMaxTimeUs = m.telemetry.NewSimpleGauge(
		subsystem, "injection_max_time_us",
		"Maximum injection time in microseconds")

	m.counters.InjectionSuccesses = m.telemetry.NewSimpleGauge(
		subsystem, "injection_successes",
		"Number of successful injections")

	m.counters.InjectionFailures = m.telemetry.NewSimpleGauge(
		subsystem, "injection_failures",
		"Number of failed injections")

	m.counters.PeCachingFailures = m.telemetry.NewSimpleGauge(
		subsystem, "pe_caching_failures",
		"Number of PE caching failures")

	m.counters.ImportDirectoryRestorationFailures = m.telemetry.NewSimpleGauge(
		subsystem, "import_directory_restoration_failures",
		"Number of import directory restoration failures")

	m.counters.PeMemoryAllocationFailures = m.telemetry.NewSimpleGauge(
		subsystem, "pe_memory_allocation_failures",
		"Number of PE memory allocation failures")

	m.counters.PeInjectionContextAllocated = m.telemetry.NewSimpleGauge(
		subsystem, "pe_injection_context_allocated",
		"Number of PE injection contexts allocated")

	m.counters.PeInjectionContextCleanedup = m.telemetry.NewSimpleGauge(
		subsystem, "pe_injection_context_cleanedup",
		"Number of PE injection contexts cleaned up")
}

// Describe implements prometheus.Collector - no-op for dynamic metrics
func (m *injectorModule) Describe(_ chan<- *prometheus.Desc) {
}

// Collect implements prometheus.Collector. Fetches stats from the injector.
func (m *injectorModule) Collect(_ chan<- prometheus.Metric) {
	if m.telemetry == nil {
		return
	}

	// Protect against concurrent collect calls from multiple sources:
	// 1. Telemetry scheduler
	// 2. RAR queries
	// 3. /telemetry endpoint
	// 4. Prometheus scraper
	m.collectMutex.Lock()
	defer m.collectMutex.Unlock()

	// Soft limiter to prevent spamming queries to the injector driver.
	elapsed := time.Since(time.Unix(m.lastCheck.Load(), 0))
	if elapsed < minInjectorQueryDelay {
		return
	}

	m.lastCheck.Store(time.Now().Unix())

	log.Debug("Collecting telemetry from Windows Injector")

	// Query the driver for current counters
	injector, err := ddinjector.NewInjector()
	if err != nil {
		log.Errorf("unable to open Windows Injector: %v", err)
		return
	}
	defer injector.Close()

	err = injector.GetCounters(&m.counters)
	if err != nil {
		log.Errorf("error getting Injector counters: %v", err)
		return
	}
}

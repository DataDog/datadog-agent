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

	"github.com/prometheus/client_golang/prometheus"

	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/windowsdriver/ddinjector"
)

const (
	// minInjectorQueryDelay is the minimum delay between queries to the injector driver.
	minInjectorQueryDelay = 5 * time.Second

	// unknownDriverVersion tags the injector metrics until the driver version has been read.
	unknownDriverVersion = "unknown"
)

func init() { registerModule(Injector) }

var _ module.Module = &injectorModule{}

// Injector Factory
var Injector = &module.Factory{
	Name: config.InjectorModule,
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
	lastCheck         atomic.Int64
	collectMutex      sync.Mutex
	telemetry         telemetry.Component
	counters          ddinjector.InjectorCounters
	sysProbeConfig    sysprobeconfig.Component
	lastOpenErrorTime atomic.Int64

	// gauges back the counters, tagged with driverVersion. Both are guarded by collectMutex.
	gauges        []versionedGauge
	driverVersion string
}

// versionedGauge binds a counter to the series of its gauge for the current driver version.
type versionedGauge struct {
	gauge   telemetry.Gauge
	counter *telemetry.SimpleGauge
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

	// Collect rebinds the counters when the driver version changes.
	m.collectMutex.Lock()
	defer m.collectMutex.Unlock()

	// If last_check_timestamp is 0, the Collect callback has not yet been trigered.
	return map[string]interface{}{
		"last_check_timestamp":                       m.lastCheck.Load(),
		"driver_version":                             m.driverVersion,
		"processes_added_to_injection_tracker":       m.counters.ProcessesAddedToInjectionTracker.Get(),
		"processes_removed_from_injection_tracker":   m.counters.ProcessesRemovedFromInjectionTracker.Get(),
		"processes_skipped_subsystem":                m.counters.ProcessesSkippedSubsystem.Get(),
		"processes_skipped_container":                m.counters.ProcessesSkippedContainer.Get(),
		"processes_skipped_protected":                m.counters.ProcessesSkippedProtected.Get(),
		"processes_skipped_system":                   m.counters.ProcessesSkippedSystem.Get(),
		"processes_skipped_excluded":                 m.counters.ProcessesSkippedExcluded.Get(),
		"injection_attempts":                         m.counters.InjectionAttempts.Get(),
		"injection_attempt_failures":                 m.counters.InjectionAttemptFailures.Get(),
		"injection_max_time_us":                      m.counters.InjectionMaxTimeUs.Get(),
		"injection_successes":                        m.counters.InjectionSuccesses.Get(),
		"injection_failures":                         m.counters.InjectionFailures.Get(),
		"pe_caching_failures":                        m.counters.PeCachingFailures.Get(),
		"import_directory_restoration_failures":      m.counters.ImportDirectoryRestorationFailures.Get(),
		"pe_memory_allocation_failures":              m.counters.PeMemoryAllocationFailures.Get(),
		"pe_injection_context_allocated":             m.counters.PeInjectionContextAllocated.Get(),
		"pe_injection_context_cleanedup":             m.counters.PeInjectionContextCleanedup.Get(),
		"crashes_during_injection":                   m.counters.CrashesDuringInjection.Get(),
		"crashes_post_injection":                     m.counters.CrashesPostInjection.Get(),
		"boot_recovery_crash_boots_detected":         m.counters.BootRecoveryCrashBootsDetected.Get(),
		"boot_recovery_driver_self_disabled":         m.counters.BootRecoveryDriverSelfDisabled.Get(),
		"boot_recovery_stability_timer_fired":        m.counters.BootRecoveryStabilityTimerFired.Get(),
		"processes_automatically_added_to_skip_list": m.counters.ProcessesAutomaticallyAddedToSkipList.Get(),
		"processes_skipped_default":                  m.counters.ProcessesSkippedDefault.Get(),
		"processes_skipped_custom":                   m.counters.ProcessesSkippedCustom.Get(),
		"processes_skipped_crash_recovery":           m.counters.ProcessesSkippedCrashRecovery.Get(),
		"processes_skipped_signature_policy":         m.counters.ProcessesSkippedSignaturePolicy.Get(),
		"processes_signature_policy_unknown":         m.counters.ProcessesSignaturePolicyUnknown.Get(),
		"processes_skipped_short_name":               m.counters.ProcessesSkippedShortName.Get(),
	}
}

////////////////////////////////////////////////////
// RAR/prometheus/telemetry related implementations

func (m *injectorModule) initializeMetrics() {
	const subsystem = "injector"
	m.driverVersion = unknownDriverVersion

	m.newVersionedGauge(&m.counters.ProcessesAddedToInjectionTracker,
		subsystem, "processes_added_to_injection_tracker",
		"Number of processes added to injection tracker")

	m.newVersionedGauge(&m.counters.ProcessesRemovedFromInjectionTracker,
		subsystem, "processes_removed_from_injection_tracker",
		"Number of processes removed from injection tracker")

	m.newVersionedGauge(&m.counters.ProcessesSkippedSubsystem,
		subsystem, "processes_skipped_subsystem",
		"Number of skipped subsystem processes")

	m.newVersionedGauge(&m.counters.ProcessesSkippedContainer,
		subsystem, "processes_skipped_container",
		"Number of skipped container processes")

	m.newVersionedGauge(&m.counters.ProcessesSkippedProtected,
		subsystem, "processes_skipped_protected",
		"Number of skipped protected processes")

	m.newVersionedGauge(&m.counters.ProcessesSkippedSystem,
		subsystem, "processes_skipped_system",
		"Number of skipped system processes")

	m.newVersionedGauge(&m.counters.ProcessesSkippedExcluded,
		subsystem, "processes_skipped_excluded",
		"Number of skipped processes due to exclusion")

	m.newVersionedGauge(&m.counters.InjectionAttempts,
		subsystem, "injection_attempts",
		"Number of injection attempts")

	m.newVersionedGauge(&m.counters.InjectionAttemptFailures,
		subsystem, "injection_attempt_failures",
		"Number of injection attempt failures")

	m.newVersionedGauge(&m.counters.InjectionMaxTimeUs,
		subsystem, "injection_max_time_us",
		"Maximum injection time in microseconds")

	m.newVersionedGauge(&m.counters.InjectionSuccesses,
		subsystem, "injection_successes",
		"Number of successful injections")

	m.newVersionedGauge(&m.counters.InjectionFailures,
		subsystem, "injection_failures",
		"Number of failed injections")

	m.newVersionedGauge(&m.counters.PeCachingFailures,
		subsystem, "pe_caching_failures",
		"Number of PE caching failures")

	m.newVersionedGauge(&m.counters.ImportDirectoryRestorationFailures,
		subsystem, "import_directory_restoration_failures",
		"Number of import directory restoration failures")

	m.newVersionedGauge(&m.counters.PeMemoryAllocationFailures,
		subsystem, "pe_memory_allocation_failures",
		"Number of PE memory allocation failures")

	m.newVersionedGauge(&m.counters.PeInjectionContextAllocated,
		subsystem, "pe_injection_context_allocated",
		"Number of PE injection contexts allocated")

	m.newVersionedGauge(&m.counters.PeInjectionContextCleanedup,
		subsystem, "pe_injection_context_cleanedup",
		"Number of PE injection contexts cleaned up")

	// v2 counters: crash / boot-recovery telemetry. Registered unconditionally;
	// they remain 0 against V1-only drivers.
	m.newVersionedGauge(&m.counters.CrashesDuringInjection,
		subsystem, "crashes_during_injection",
		"Number of crashes observed during injection")

	m.newVersionedGauge(&m.counters.CrashesPostInjection,
		subsystem, "crashes_post_injection",
		"Number of crashes observed after injection")

	m.newVersionedGauge(&m.counters.BootRecoveryCrashBootsDetected,
		subsystem, "boot_recovery_crash_boots_detected",
		"Number of crash boots detected by boot recovery")

	m.newVersionedGauge(&m.counters.BootRecoveryDriverSelfDisabled,
		subsystem, "boot_recovery_driver_self_disabled",
		"Number of times the driver self-disabled via boot recovery")

	m.newVersionedGauge(&m.counters.BootRecoveryStabilityTimerFired,
		subsystem, "boot_recovery_stability_timer_fired",
		"Number of times the boot recovery stability timer fired")

	// v3 counters: skip-list sources and signature policy. Registered
	// unconditionally; they remain 0 against drivers older than V3.
	m.newVersionedGauge(&m.counters.ProcessesAutomaticallyAddedToSkipList,
		subsystem, "processes_automatically_added_to_skip_list",
		"Number of processes automatically added to the skip list by crash recovery")

	m.newVersionedGauge(&m.counters.ProcessesSkippedDefault,
		subsystem, "processes_skipped_default",
		"Number of processes skipped due to the default skip list")

	m.newVersionedGauge(&m.counters.ProcessesSkippedCustom,
		subsystem, "processes_skipped_custom",
		"Number of processes skipped due to the custom skip list")

	m.newVersionedGauge(&m.counters.ProcessesSkippedCrashRecovery,
		subsystem, "processes_skipped_crash_recovery",
		"Number of processes skipped due to the crash-recovery skip list")

	m.newVersionedGauge(&m.counters.ProcessesSkippedSignaturePolicy,
		subsystem, "processes_skipped_signature_policy",
		"Number of processes skipped due to a binary signature policy")

	m.newVersionedGauge(&m.counters.ProcessesSignaturePolicyUnknown,
		subsystem, "processes_signature_policy_unknown",
		"Number of processes whose binary signature policy could not be determined")

	m.newVersionedGauge(&m.counters.ProcessesSkippedShortName,
		subsystem, "processes_skipped_short_name",
		"Number of processes skipped because their image path kept an 8.3 short name")
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

	currentTime := time.Now().Unix()
	m.lastCheck.Store(currentTime)

	log.Debug("Collecting telemetry from Windows Injector")

	// Query the driver for current counters
	injector, err := ddinjector.NewInjector()
	if err != nil {
		if m.lastOpenErrorTime.Load() == 0 {
			// If we cannot open ddinjector, most likely it is disabled. Do not spam the log.
			log.Warnf("failed to open Windows Injector the first time, ignoring future errors: %v", err)
			m.lastOpenErrorTime.Store(currentTime)
		}

		return
	} else if m.lastOpenErrorTime.Load() != 0 {
		// We recovered from a previous error. Log that we are collecting telemetry again.
		log.Info("regained access to Windows Injector, collecting telemetry again")
		m.lastOpenErrorTime.Store(0)
	}
	defer injector.Close()

	version, err := ddinjector.DriverVersion()
	if err != nil {
		log.Debugf("error getting Injector driver version: %v", err)
		version = unknownDriverVersion
	}
	m.setDriverVersion(version)

	err = injector.GetCounters(&m.counters)
	if err != nil {
		log.Errorf("error getting Injector counters: %v", err)
		return
	}
}

// newVersionedGauge creates a gauge tagged with the driver version and binds counter to its
// series for the current version.
func (m *injectorModule) newVersionedGauge(counter *telemetry.SimpleGauge, subsystem, name, help string) {
	gauge := m.telemetry.NewGauge(subsystem, name, []string{"ddinjector_version"}, help)
	*counter = gauge.WithValues(m.driverVersion)
	m.gauges = append(m.gauges, versionedGauge{gauge: gauge, counter: counter})
}

// setDriverVersion rebinds the counters to the series of version, deleting the series of the
// previous version so a driver upgrade does not leave stale ones behind. The new series start
// at 0 until the next counters query. Must be called with collectMutex held.
func (m *injectorModule) setDriverVersion(version string) {
	if version == m.driverVersion {
		return
	}
	for _, g := range m.gauges {
		g.gauge.Delete(m.driverVersion)
		*g.counter = g.gauge.WithValues(version)
	}
	m.driverVersion = version
}

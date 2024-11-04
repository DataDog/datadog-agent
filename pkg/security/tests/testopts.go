// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests || stresstests

// Package tests holds tests related files
package tests

import (
	"reflect"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

type testOpts struct {
	disableFilters                             bool
	disableApprovers                           bool
	disableEnvVarsResolution                   bool
	enableActivityDump                         bool
	activityDumpRateLimiter                    int
	activityDumpTagRules                       bool
	activityDumpDuration                       time.Duration
	activityDumpLoadControllerPeriod           time.Duration
	activityDumpCleanupPeriod                  time.Duration
	activityDumpLoadControllerTimeout          time.Duration
	activityDumpTracedCgroupsCount             int
	activityDumpCgroupDifferentiateArgs        bool
	activityDumpAutoSuppressionEnabled         bool
	activityDumpTracedEventTypes               []string
	activityDumpLocalStorageDirectory          string
	activityDumpLocalStorageCompression        bool
	activityDumpLocalStorageFormats            []string
	activityDumpSyscallMonitorPeriod           time.Duration
	enableSecurityProfile                      bool
	securityProfileMaxImageTags                int
	securityProfileDir                         string
	securityProfileWatchDir                    bool
	enableAutoSuppression                      bool
	autoSuppressionEventTypes                  []string
	enableAnomalyDetection                     bool
	anomalyDetectionEventTypes                 []string
	anomalyDetectionDefaultMinimumStablePeriod time.Duration
	anomalyDetectionMinimumStablePeriodExec    time.Duration
	anomalyDetectionMinimumStablePeriodDNS     time.Duration
	anomalyDetectionWarmupPeriod               time.Duration
	disableDiscarders                          bool
	disableERPCDentryResolution                bool
	disableMapDentryResolution                 bool
	envsWithValue                              []string
	disableRuntimeSecurity                     bool
	enableSBOM                                 bool
	enableHostSBOM                             bool
	preStartCallback                           func(test *testModule)
	tagsResolver                               tags.Resolver
	snapshotRuleMatchHandler                   func(*testModule, *model.Event, *rules.Rule)
	enableFIM                                  bool // only valid on windows
	networkIngressEnabled                      bool
	networkRawPacketEnabled                    bool
	disableOnDemandRateLimiter                 bool
	ebpfLessEnabled                            bool
	dontWaitEBPFLessClient                     bool
	enforcementExcludeBinary                   string
	enforcementDisarmerContainerEnabled        bool
	enforcementDisarmerContainerMaxAllowed     int
	enforcementDisarmerContainerPeriod         time.Duration
	enforcementDisarmerExecutableEnabled       bool
	enforcementDisarmerExecutableMaxAllowed    int
	enforcementDisarmerExecutablePeriod        time.Duration
	eventServerRetention                       time.Duration
	discardRuntime                             bool
}

type dynamicTestOpts struct {
	testDir                  string
	disableAbnormalPathCheck bool
	disableBundledRules      bool
}

type tmOpts struct {
	staticOpts  testOpts
	dynamicOpts dynamicTestOpts
	forceReload bool
}

type optFunc = func(opts *tmOpts)

func withStaticOpts(opts testOpts) optFunc {
	return func(tmo *tmOpts) {
		tmo.staticOpts = opts
	}
}

func withDynamicOpts(opts dynamicTestOpts) optFunc {
	return func(tmo *tmOpts) {
		tmo.dynamicOpts = opts
	}
}

func withForceReload() optFunc {
	return func(tmo *tmOpts) {
		tmo.forceReload = true
	}
}

func (to testOpts) Equal(opts testOpts) bool {
	return to.disableApprovers == opts.disableApprovers &&
		to.disableEnvVarsResolution == opts.disableEnvVarsResolution &&
		to.enableActivityDump == opts.enableActivityDump &&
		to.activityDumpRateLimiter == opts.activityDumpRateLimiter &&
		to.activityDumpTagRules == opts.activityDumpTagRules &&
		to.activityDumpDuration == opts.activityDumpDuration &&
		to.activityDumpLoadControllerPeriod == opts.activityDumpLoadControllerPeriod &&
		to.activityDumpTracedCgroupsCount == opts.activityDumpTracedCgroupsCount &&
		to.activityDumpCgroupDifferentiateArgs == opts.activityDumpCgroupDifferentiateArgs &&
		to.activityDumpAutoSuppressionEnabled == opts.activityDumpAutoSuppressionEnabled &&
		to.activityDumpLoadControllerTimeout == opts.activityDumpLoadControllerTimeout &&
		to.activityDumpSyscallMonitorPeriod == opts.activityDumpSyscallMonitorPeriod &&
		reflect.DeepEqual(to.activityDumpTracedEventTypes, opts.activityDumpTracedEventTypes) &&
		to.activityDumpLocalStorageDirectory == opts.activityDumpLocalStorageDirectory &&
		to.activityDumpLocalStorageCompression == opts.activityDumpLocalStorageCompression &&
		reflect.DeepEqual(to.activityDumpLocalStorageFormats, opts.activityDumpLocalStorageFormats) &&
		to.enableSecurityProfile == opts.enableSecurityProfile &&
		to.securityProfileMaxImageTags == opts.securityProfileMaxImageTags &&
		to.securityProfileDir == opts.securityProfileDir &&
		to.securityProfileWatchDir == opts.securityProfileWatchDir &&
		to.enableAutoSuppression == opts.enableAutoSuppression &&
		slices.Equal(to.autoSuppressionEventTypes, opts.autoSuppressionEventTypes) &&
		to.enableAnomalyDetection == opts.enableAnomalyDetection &&
		slices.Equal(to.anomalyDetectionEventTypes, opts.anomalyDetectionEventTypes) &&
		to.anomalyDetectionDefaultMinimumStablePeriod == opts.anomalyDetectionDefaultMinimumStablePeriod &&
		to.anomalyDetectionMinimumStablePeriodExec == opts.anomalyDetectionMinimumStablePeriodExec &&
		to.anomalyDetectionMinimumStablePeriodDNS == opts.anomalyDetectionMinimumStablePeriodDNS &&
		to.anomalyDetectionWarmupPeriod == opts.anomalyDetectionWarmupPeriod &&
		to.disableDiscarders == opts.disableDiscarders &&
		to.disableFilters == opts.disableFilters &&
		to.disableERPCDentryResolution == opts.disableERPCDentryResolution &&
		to.disableMapDentryResolution == opts.disableMapDentryResolution &&
		reflect.DeepEqual(to.envsWithValue, opts.envsWithValue) &&
		to.disableRuntimeSecurity == opts.disableRuntimeSecurity &&
		to.enableSBOM == opts.enableSBOM &&
		to.enableHostSBOM == opts.enableHostSBOM &&
		to.snapshotRuleMatchHandler == nil && opts.snapshotRuleMatchHandler == nil &&
		to.preStartCallback == nil && opts.preStartCallback == nil &&
		to.networkIngressEnabled == opts.networkIngressEnabled &&
		to.networkRawPacketEnabled == opts.networkRawPacketEnabled &&
		to.disableOnDemandRateLimiter == opts.disableOnDemandRateLimiter &&
		to.ebpfLessEnabled == opts.ebpfLessEnabled &&
		to.enforcementExcludeBinary == opts.enforcementExcludeBinary &&
		to.enforcementDisarmerContainerEnabled == opts.enforcementDisarmerContainerEnabled &&
		to.enforcementDisarmerContainerMaxAllowed == opts.enforcementDisarmerContainerMaxAllowed &&
		to.enforcementDisarmerContainerPeriod == opts.enforcementDisarmerContainerPeriod &&
		to.enforcementDisarmerExecutableEnabled == opts.enforcementDisarmerExecutableEnabled &&
		to.enforcementDisarmerExecutableMaxAllowed == opts.enforcementDisarmerExecutableMaxAllowed &&
		to.enforcementDisarmerExecutablePeriod == opts.enforcementDisarmerExecutablePeriod &&
		to.eventServerRetention == opts.eventServerRetention &&
		to.discardRuntime == opts.discardRuntime
}

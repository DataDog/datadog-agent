// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

import (
	"reflect"
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
	activityDumpCleanupPeriod                  time.Duration
	activityDumpTracedCgroupsCount             int
	activityDumpCgroupDifferentiateArgs        bool
	activityDumpTracedEventTypes               []string
	activityDumpLocalStorageDirectory          string
	activityDumpLocalStorageCompression        bool
	activityDumpLocalStorageFormats            []string
	activityDumpSyscallMonitorPeriod           time.Duration
	enableSecurityProfile                      bool
	securityProfileMaxImageTags                int
	securityProfileDir                         string
	securityProfileWatchDir                    bool
	securityProfileNodeEvictionTimeout         time.Duration
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
	tagger                                     tags.Tagger
	ruleMatchHandler                           func(*testModule, *model.Event, *rules.Rule)
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
	enableSelfTests                            bool
	networkFlowMonitorEnabled                  bool
	dnsPort                                    uint16
	traceSystemdCgroups                        bool
	capabilitiesMonitoringEnabled              bool
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
	return reflect.DeepEqual(to, opts)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests || stresstests

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
	enableActivityDump                         bool
	activityDumpRateLimiter                    int
	activityDumpTagRules                       bool
	activityDumpDuration                       time.Duration
	activityDumpLoadControllerPeriod           time.Duration
	activityDumpCleanupPeriod                  time.Duration
	activityDumpLoadControllerTimeout          time.Duration
	activityDumpTracedCgroupsCount             int
	activityDumpTracedEventTypes               []string
	activityDumpLocalStorageDirectory          string
	activityDumpLocalStorageCompression        bool
	activityDumpLocalStorageFormats            []string
	enableSecurityProfile                      bool
	securityProfileDir                         string
	securityProfileWatchDir                    bool
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
	preStartCallback                           func(test *testModule)
	tagsResolver                               tags.Resolver
	snapshotRuleMatchHandler                   func(*testModule, *model.Event, *rules.Rule)
}

type dynamicTestOpts struct {
	testDir                  string
	disableAbnormalPathCheck bool
}

type tmOpts struct {
	staticOpts  testOpts
	dynamicOpts dynamicTestOpts
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
func (to testOpts) Equal(opts testOpts) bool {
	return to.disableApprovers == opts.disableApprovers &&
		to.enableActivityDump == opts.enableActivityDump &&
		to.activityDumpRateLimiter == opts.activityDumpRateLimiter &&
		to.activityDumpTagRules == opts.activityDumpTagRules &&
		to.activityDumpDuration == opts.activityDumpDuration &&
		to.activityDumpLoadControllerPeriod == opts.activityDumpLoadControllerPeriod &&
		to.activityDumpTracedCgroupsCount == opts.activityDumpTracedCgroupsCount &&
		to.activityDumpLoadControllerTimeout == opts.activityDumpLoadControllerTimeout &&
		reflect.DeepEqual(to.activityDumpTracedEventTypes, opts.activityDumpTracedEventTypes) &&
		to.activityDumpLocalStorageDirectory == opts.activityDumpLocalStorageDirectory &&
		to.activityDumpLocalStorageCompression == opts.activityDumpLocalStorageCompression &&
		reflect.DeepEqual(to.activityDumpLocalStorageFormats, opts.activityDumpLocalStorageFormats) &&
		to.enableSecurityProfile == opts.enableSecurityProfile &&
		to.securityProfileDir == opts.securityProfileDir &&
		to.securityProfileWatchDir == opts.securityProfileWatchDir &&
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
		to.snapshotRuleMatchHandler == nil && opts.snapshotRuleMatchHandler == nil &&
		to.preStartCallback == nil && opts.preStartCallback == nil
}

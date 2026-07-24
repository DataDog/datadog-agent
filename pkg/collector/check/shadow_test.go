// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewShadowCheckOverridesIdentityAndInterval(t *testing.T) {
	inner := &recordingCheck{
		id:       checkid.ID("cpu:abc123"),
		interval: 15 * time.Second,
	}

	shadow := NewShadowCheck(inner, time.Second)

	require.True(t, IsShadow(shadow))
	assert.False(t, IsShadow(inner))
	assert.Equal(t, checkid.ID("cpu:abc123:shadow"), shadow.ID())
	assert.Equal(t, time.Second, shadow.Interval())
	assert.Same(t, inner, shadow.Unwrap())
}

func TestShadowCheckIDTracksInnerCheckID(t *testing.T) {
	inner := &recordingCheck{id: checkid.ID("cpu:abc123")}
	shadow := NewShadowCheck(inner, time.Second)

	assert.Equal(t, checkid.ID("cpu:abc123:shadow"), shadow.ID())

	inner.id = checkid.ID("cpu:def456")
	assert.Equal(t, checkid.ID("cpu:def456:shadow"), shadow.ID())
}

func TestShadowCheckCanUseExplicitSourceID(t *testing.T) {
	inner := &recordingCheck{id: checkid.ID("cpu:shadow-config-digest")}
	shadow := NewShadowCheckForSource(inner, checkid.ID("cpu:source-digest"), time.Second, nil)

	assert.Equal(t, checkid.ID("cpu:source-digest:shadow"), shadow.ID())

	inner.id = checkid.ID("cpu:changed-inner-digest")
	assert.Equal(t, checkid.ID("cpu:source-digest:shadow"), shadow.ID())
}

func TestShadowCheckDelegatesInnerCheckBehavior(t *testing.T) {
	expectedErr := errors.New("run failed")
	expectedWarnings := []error{errors.New("warning")}
	expectedStats := stats.NewSenderStats()
	expectedDiagnoses := []diagnose.Diagnosis{{Diagnosis: "ok"}}
	inner := &recordingCheck{
		name:             "cpu",
		loader:           "core",
		version:          "1.2.3",
		configSource:     "file:cpu.yaml",
		configProvider:   "file",
		initConfig:       "init",
		instanceConfig:   "instance",
		telemetryEnabled: true,
		haSupported:      true,
		runErr:           expectedErr,
		warnings:         expectedWarnings,
		senderStats:      expectedStats,
		diagnoses:        expectedDiagnoses,
	}

	shadow := NewShadowCheck(inner, time.Second)

	assert.Equal(t, expectedErr, shadow.Run())
	shadow.Stop()
	shadow.Cancel()
	err := shadow.Configure(nil, 123, integration.Data("config"), integration.Data("init"), "source", "provider")
	require.NoError(t, err)

	assert.Equal(t, "cpu", shadow.String())
	assert.Equal(t, "core", shadow.Loader())
	assert.Equal(t, expectedWarnings, shadow.GetWarnings())
	gotStats, err := shadow.GetSenderStats()
	require.NoError(t, err)
	assert.Equal(t, expectedStats, gotStats)
	assert.Equal(t, "1.2.3", shadow.Version())
	assert.Equal(t, "file:cpu.yaml", shadow.ConfigSource())
	assert.Equal(t, "file", shadow.ConfigProvider())
	assert.True(t, shadow.IsTelemetryEnabled())
	assert.Equal(t, "init", shadow.InitConfig())
	assert.Equal(t, "instance", shadow.InstanceConfig())
	gotDiagnoses, err := shadow.GetDiagnoses()
	require.NoError(t, err)
	assert.Equal(t, expectedDiagnoses, gotDiagnoses)
	assert.True(t, shadow.IsHASupported())

	assert.True(t, inner.ran)
	assert.True(t, inner.stopped)
	assert.True(t, inner.cancelled)
	assert.Equal(t, uint64(123), inner.integrationConfigDigest)
	assert.Equal(t, integration.Data("config"), inner.config)
	assert.Equal(t, integration.Data("init"), inner.initConfigData)
	assert.Equal(t, "source", inner.source)
	assert.Equal(t, "provider", inner.provider)
}

func TestShadowIDAppendsShadowSuffix(t *testing.T) {
	assert.Equal(t, checkid.ID("ntp:def456:shadow"), ShadowID(checkid.ID("ntp:def456")))
}

func TestAsFindsWrappedCheckCapability(t *testing.T) {
	reporter := &recordingIssueReporter{}
	inner := &issueAwareRecordingCheck{}
	shadow := NewShadowCheck(inner, time.Second)

	aware, ok := As[IssueAwareCheck](shadow)
	require.True(t, ok)
	aware.SetIssueReporter(reporter)

	assert.Equal(t, reporter, inner.reporter)
}

func TestIsShadowUnwrapsCheckWrappers(t *testing.T) {
	inner := &recordingCheck{id: checkid.ID("cpu:abc123")}
	shadow := NewShadowCheck(inner, time.Second)

	assert.True(t, IsShadow(&recordingWrapper{Check: shadow}))
	assert.False(t, IsShadow(&recordingWrapper{Check: inner}))
}

type recordingWrapper struct {
	Check
}

func (w *recordingWrapper) Unwrap() Check {
	return w.Check
}

func TestShadowCheckExposesSenderManagerOverride(t *testing.T) {
	inner := &recordingCheck{id: checkid.ID("cpu:abc123")}
	shadowSenderManager := &recordingSenderManager{}
	shadow := NewShadowCheckWithSenderManagerOverride(inner, time.Second, shadowSenderManager)

	gotSenderManager, ok := SenderManagerOverride(shadow)
	require.True(t, ok)
	assert.Same(t, shadowSenderManager, gotSenderManager)
}

func TestSenderManagerOverrideIgnoresNormalChecksAndUnsetShadowChecks(t *testing.T) {
	inner := &recordingCheck{id: checkid.ID("cpu:abc123")}
	shadow := NewShadowCheck(inner, time.Second)

	_, ok := SenderManagerOverride(inner)
	assert.False(t, ok)

	_, ok = SenderManagerOverride(shadow)
	assert.False(t, ok)
}

type recordingCheck struct {
	id       checkid.ID
	interval time.Duration

	name             string
	loader           string
	version          string
	configSource     string
	configProvider   string
	initConfig       string
	instanceConfig   string
	telemetryEnabled bool
	haSupported      bool

	runErr      error
	warnings    []error
	senderStats stats.SenderStats
	diagnoses   []diagnose.Diagnosis

	ran       bool
	stopped   bool
	cancelled bool

	senderManager           sender.SenderManager
	integrationConfigDigest uint64
	config                  integration.Data
	initConfigData          integration.Data
	source                  string
	provider                string
}

func (c *recordingCheck) Run() error {
	c.ran = true
	return c.runErr
}

func (c *recordingCheck) Stop() {
	c.stopped = true
}

func (c *recordingCheck) Cancel() {
	c.cancelled = true
}

func (c *recordingCheck) String() string {
	return c.name
}

func (c *recordingCheck) Loader() string {
	return c.loader
}

func (c *recordingCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, config, initConfig integration.Data, source string, provider string) error {
	c.senderManager = senderManager
	c.integrationConfigDigest = integrationConfigDigest
	c.config = config
	c.initConfigData = initConfig
	c.source = source
	c.provider = provider
	return nil
}

func (c *recordingCheck) Interval() time.Duration {
	return c.interval
}

func (c *recordingCheck) ID() checkid.ID {
	return c.id
}

func (c *recordingCheck) GetWarnings() []error {
	return c.warnings
}

func (c *recordingCheck) GetSenderStats() (stats.SenderStats, error) {
	return c.senderStats, nil
}

func (c *recordingCheck) Version() string {
	return c.version
}

func (c *recordingCheck) ConfigSource() string {
	return c.configSource
}

func (c *recordingCheck) ConfigProvider() string {
	return c.configProvider
}

func (c *recordingCheck) IsTelemetryEnabled() bool {
	return c.telemetryEnabled
}

func (c *recordingCheck) InitConfig() string {
	return c.initConfig
}

func (c *recordingCheck) InstanceConfig() string {
	return c.instanceConfig
}

func (c *recordingCheck) GetDiagnoses() ([]diagnose.Diagnosis, error) {
	return c.diagnoses, nil
}

func (c *recordingCheck) IsHASupported() bool {
	return c.haSupported
}

type recordingIssueReporter struct {
	healthplatformstore.Component
}

type issueAwareRecordingCheck struct {
	recordingCheck
	reporter healthplatformstore.Component
}

func (c *issueAwareRecordingCheck) SetIssueReporter(reporter healthplatformstore.Component) {
	c.reporter = reporter
}

type recordingSenderManager struct {
	sender.SenderManager
}

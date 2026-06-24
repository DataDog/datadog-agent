// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package check contains the interface for the check.
package check

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
)

// ShadowableCheck is implemented by checks that can create a shadow copy of themselves.
type ShadowableCheck interface {
	ShadowCheck() (Check, error)
}

// ShadowCheck wraps an inner Check and delegates all method calls to it.
type ShadowCheck struct {
	inner  Check
	config ShadowConfig
}

type ShadowConfig struct {
	Interval time.Duration
}

// NewShadowCheck creates a new ShadowCheck wrapping the given inner check.
func NewShadowCheck[T Check](inner T, config ShadowConfig) Check {
	return &ShadowCheck{inner: inner, config: config}
}

func (s *ShadowCheck) Run() error {
	return s.inner.Run()
}

func (s *ShadowCheck) Stop() {
	s.inner.Stop()
}

func (s *ShadowCheck) Cancel() {
	s.inner.Cancel()
}

func (s *ShadowCheck) String() string {
	return s.inner.String()
}

func (s *ShadowCheck) Loader() string {
	return s.inner.Loader()
}

func (s *ShadowCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, config, initConfig integration.Data, source string, provider string) error {
	return s.inner.Configure(senderManager, integrationConfigDigest, config, initConfig, source, provider)
}

func (s *ShadowCheck) Interval() time.Duration {
	return s.config.Interval
}

func (s *ShadowCheck) ID() checkid.ID {
	return s.inner.ID() + ":shadow"
}

func (s *ShadowCheck) GetWarnings() []error {
	return s.inner.GetWarnings()
}

func (s *ShadowCheck) GetSenderStats() (stats.SenderStats, error) {
	return s.inner.GetSenderStats()
}

func (s *ShadowCheck) Version() string {
	return s.inner.Version()
}

func (s *ShadowCheck) ConfigSource() string {
	return s.inner.ConfigSource()
}

func (s *ShadowCheck) ConfigProvider() string {
	return s.inner.ConfigProvider()
}

func (s *ShadowCheck) IsTelemetryEnabled() bool {
	return s.inner.IsTelemetryEnabled()
}

func (s *ShadowCheck) InitConfig() string {
	return s.inner.InitConfig()
}

func (s *ShadowCheck) InstanceConfig() string {
	return s.inner.InstanceConfig()
}

func (s *ShadowCheck) GetDiagnoses() ([]diagnose.Diagnosis, error) {
	return s.inner.GetDiagnoses()
}

func (s *ShadowCheck) IsHASupported() bool {
	return s.inner.IsHASupported()
}

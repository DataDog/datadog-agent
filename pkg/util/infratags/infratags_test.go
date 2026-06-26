// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infratags

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
)

func TestIsTagged(t *testing.T) {
	tests := []struct {
		name         string
		checkName    string
		taggedChecks []string
		wantResult   bool
	}{
		{"check in allow-list returns true", "cpu", []string{"cpu"}, true},
		{"check not in allow-list returns false", "disk", []string{"cpu"}, false},
		{"empty allow-list tags all non-custom checks", "any_check", nil, true},
		{"custom check is never tagged", "custom_my", nil, false},
		{"empty check name returns false", "", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantResult, IsTagged(tt.checkName, tt.taggedChecks))
		})
	}
}

func TestResolveEnrichmentState(t *testing.T) {
	tests := []struct {
		name             string
		mode             string
		taggedChecks     []string
		wantTags         []string
		wantTaggedChecks []string
	}{
		{"cloud_cost_only returns tags and empty list", "cloud_cost_only", []string{}, []string{InfraModeCloudCostTag}, []string{}},
		{"cloud_cost_only returns tags and allow-list", "cloud_cost_only", []string{"cpu"}, []string{InfraModeCloudCostTag}, []string{"cpu"}},
		{"full returns nil tags", "full", nil, nil, nil},
		{"unknown mode returns nil tags", "some_future_mode", nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.Set("infrastructure_mode", tt.mode, pkgconfigmodel.SourceFile)
			if tt.taggedChecks != nil {
				cfg.Set("integration."+tt.mode+".tagged", tt.taggedChecks, pkgconfigmodel.SourceFile)
			}
			tags, taggedChecks := ResolveEnrichmentState(cfg)
			assert.Equal(t, tt.wantTags, tags)
			assert.Equal(t, tt.wantTaggedChecks, taggedChecks)
		})
	}
}

func TestAppendJMXDogstatsdInfraTags(t *testing.T) {
	cloudCostTags := []string{InfraModeCloudCostTag}

	t.Run("empty infraModeTags is no-op", func(t *testing.T) {
		tags := []string{"a:1"}
		assert.Equal(t, tags, AppendJMXDogstatsdInfraTags(tags, "kafka", nil, nil))
	})
	t.Run("empty jmxCheckName is no-op", func(t *testing.T) {
		tags := []string{"env:prod"}
		assert.Equal(t, tags, AppendJMXDogstatsdInfraTags(tags, "", cloudCostTags, nil))
	})
	t.Run("eligible JMX check gets tags", func(t *testing.T) {
		tags := []string{"env:prod"}
		got := AppendJMXDogstatsdInfraTags(tags, "kafka", cloudCostTags, []string{"kafka"})
		assert.Contains(t, got, InfraModeCloudCostTag)
	})
	t.Run("JMX check not in tagged list", func(t *testing.T) {
		tags := []string{"env:prod"}
		got := AppendJMXDogstatsdInfraTags(tags, "tomcat", cloudCostTags, []string{"kafka"})
		assert.NotContains(t, got, InfraModeCloudCostTag)
	})
	t.Run("empty tagged list tags all checks", func(t *testing.T) {
		tags := []string{"env:prod"}
		got := AppendJMXDogstatsdInfraTags(tags, "kafka", cloudCostTags, nil)
		assert.Contains(t, got, InfraModeCloudCostTag)
	})
	t.Run("custom_ JMX check name is not tagged", func(t *testing.T) {
		tags := []string{"env:prod"}
		got := AppendJMXDogstatsdInfraTags(tags, "custom_jmx", cloudCostTags, nil)
		assert.NotContains(t, got, InfraModeCloudCostTag)
	})
}

func TestApplySenderTags(t *testing.T) {
	tests := []struct {
		name          string
		integration   string
		setupCfg      func(cfg pkgconfigmodel.Config)
		getSenderErr  error
		wantGetSender bool
		wantInfraTags [][]string
	}{
		{
			name:        "eligible integration appends infra_mode tag",
			integration: "cpu",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("infrastructure_mode", "cloud_cost_only", pkgconfigmodel.SourceFile)
				cfg.Set("integration.cloud_cost_only.tagged", []string{"cpu"}, pkgconfigmodel.SourceFile)
			},
			wantGetSender: true,
			wantInfraTags: [][]string{{"infra_mode:cloud_cost_only"}},
		},
		{
			name:        "custom check is never tagged via sender",
			integration: "custom_foo",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("infrastructure_mode", "cloud_cost_only", pkgconfigmodel.SourceFile)
				cfg.Set("integration.cloud_cost_only.tagged", []string{}, pkgconfigmodel.SourceFile)
			},
			wantGetSender: false,
		},
		{
			name:        "integration not in tagged list skips tagging",
			integration: "disk",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("infrastructure_mode", "cloud_cost_only", pkgconfigmodel.SourceFile)
				cfg.Set("integration.cloud_cost_only.tagged", []string{"cpu"}, pkgconfigmodel.SourceFile)
			},
			wantGetSender: false,
		},
		{
			name:        "non-taggable mode skips tagging",
			integration: "cpu",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("infrastructure_mode", "full", pkgconfigmodel.SourceFile)
				cfg.Set("integration.cloud_cost_only.tagged", []string{"cpu"}, pkgconfigmodel.SourceFile)
			},
			wantGetSender: false,
		},
		{
			name:        "GetSender error skips tagging",
			integration: "cpu",
			setupCfg: func(cfg pkgconfigmodel.Config) {
				cfg.Set("infrastructure_mode", "cloud_cost_only", pkgconfigmodel.SourceFile)
				cfg.Set("integration.cloud_cost_only.tagged", []string{"cpu"}, pkgconfigmodel.SourceFile)
			},
			getSenderErr:  errors.New("sender not found"),
			wantGetSender: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spy := &spySender{}
			mgr := &stubSenderManager{sender: spy, getSenderErr: tt.getSenderErr}

			cfg := configmock.New(t)
			tt.setupCfg(cfg)

			ApplySenderTags(mgr, checkid.ID("test:id"), tt.integration, cfg)

			assert.Equal(t, tt.wantGetSender, mgr.getSenderCalled)
			assert.Equal(t, tt.wantInfraTags, spy.infraTags)
		})
	}
}

type stubSenderManager struct {
	sender          sender.Sender
	getSenderErr    error
	getSenderCalled bool
}

func (m *stubSenderManager) GetSender(checkid.ID) (sender.Sender, error) {
	m.getSenderCalled = true
	if m.getSenderErr != nil {
		return nil, m.getSenderErr
	}
	return m.sender, nil
}

func (m *stubSenderManager) SetSender(sender.Sender, checkid.ID) error { return nil }
func (m *stubSenderManager) DestroySender(checkid.ID)                  {}
func (m *stubSenderManager) GetDefaultSender() (sender.Sender, error) {
	return nil, errors.New("not implemented")
}

type spySender struct {
	noopSender
	infraTags [][]string
}

func (s *spySender) AppendInfraTags(tags []string) {
	s.infraTags = append(s.infraTags, tags)
}

type noopSender struct{}

func (noopSender) Commit() {}
func (noopSender) Gauge(string, float64, string, []string) {
}
func (noopSender) GaugeNoIndex(string, float64, string, []string)   {}
func (noopSender) Rate(string, float64, string, []string)           {}
func (noopSender) Count(string, float64, string, []string)          {}
func (noopSender) MonotonicCount(string, float64, string, []string) {}
func (noopSender) MonotonicCountWithFlushFirstValue(string, float64, string, []string, bool) {
}
func (noopSender) Counter(string, float64, string, []string)      {}
func (noopSender) Histogram(string, float64, string, []string)    {}
func (noopSender) Historate(string, float64, string, []string)    {}
func (noopSender) Distribution(string, float64, string, []string) {}
func (noopSender) ServiceCheck(string, servicecheck.ServiceCheckStatus, string, []string, string) {
}
func (noopSender) OpenmetricsBucket(string, int64, float64, float64, bool, string, []string, bool) {
}
func (noopSender) HistogramBucket(string, int64, float64, float64, bool, string, []string, bool) {
}
func (noopSender) GaugeWithTimestamp(string, float64, string, []string, float64) error {
	return nil
}
func (noopSender) CountWithTimestamp(string, float64, string, []string, float64) error {
	return nil
}
func (noopSender) Event(event.Event)                 {}
func (noopSender) EventPlatformEvent([]byte, string) {}
func (noopSender) GetSenderStats() stats.SenderStats { return stats.SenderStats{} }
func (noopSender) DisableDefaultHostname(bool)       {}
func (noopSender) SetCheckCustomTags([]string)       {}
func (noopSender) AppendInfraTags([]string)          {}
func (noopSender) SetCheckService(string)            {}
func (noopSender) SetNoIndex(bool)                   {}
func (noopSender) FinalizeCheckServiceTag()          {}
func (noopSender) OrchestratorMetadata([]types.ProcessMessageBody, string, int) {
}
func (noopSender) OrchestratorManifest([]types.ProcessMessageBody, string) {}

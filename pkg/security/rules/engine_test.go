// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// This test is less important now that remoteConfigProvidersFirst() exists, which enforces that the RC providers are first
func TestRuleEngineGatherPolicyProviders(t *testing.T) {
	type fields struct {
		config *config.RuntimeSecurityConfig
	}
	tests := []struct {
		name     string
		fields   fields
		wantType string
		wantLen  int
	}{
		{
			name:     "RC enabled",
			fields:   fields{config: &config.RuntimeSecurityConfig{RemoteConfigurationEnabled: true}},
			wantType: rules.PolicyProviderTypeRC,
			wantLen:  3,
		},
		{
			name:     "RC disabled",
			fields:   fields{config: &config.RuntimeSecurityConfig{RemoteConfigurationEnabled: false}},
			wantType: rules.PolicyProviderTypeDir,
			wantLen:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &RuleEngine{
				config: tt.fields.config,
				ipc:    ipcmock.New(t),
			}

			got := e.gatherDefaultPolicyProviders()

			assert.Equal(t, tt.wantLen, len(got))
			assert.Equal(t, tt.wantType, got[1].Type())
		})
	}
}

type countCall struct {
	name string
	val  int64
	tags []string
}

type captureStatsdClient struct {
	statsd.NoOpClient
	calls []countCall
}

func (c *captureStatsdClient) Count(name string, value int64, tags []string, _ float64) error {
	c.calls = append(c.calls, countCall{name: name, val: value, tags: append([]string(nil), tags...)})
	return nil
}

func TestRuleEngineNoMatchMetric(t *testing.T) {
	client := &captureStatsdClient{}

	enabled := map[eval.EventType]bool{"*": true}
	ruleOpts, evalOpts := rules.NewBothOpts(enabled)
	rs := rules.NewRuleSet(&model.Model{}, func() eval.Event { return model.NewFakeEvent() }, ruleOpts, evalOpts)

	engine := &RuleEngine{
		config:         &config.RuntimeSecurityConfig{},
		probe:          &probe.Probe{Opts: probe.Opts{DontDiscardRuntime: true}},
		statsdClient:   client,
		currentRuleSet: new(atomic.Value),
		reloading:      atomic.NewBool(false),
	}
	engine.noMatchCounters = make([]atomic.Uint64, model.MaxAllEventType)
	engine.currentRuleSet.Store(rs)

	ev := model.NewFakeEvent()
	ev.Type = uint32(model.ExecEventType)
	engine.HandleEvent(ev)

	engine.SendStats()

	var matched []countCall
	for _, call := range client.calls {
		if call.name == metrics.MetricRulesNoMatch {
			matched = append(matched, call)
		}
	}

	if assert.Len(t, matched, 1) {
		assert.Equal(t, int64(1), matched[0].val)
		assert.Contains(t, matched[0].tags, "event_type:exec")
		assert.Contains(t, matched[0].tags, "category:"+model.GetEventTypeCategory("exec").String())
	}
}

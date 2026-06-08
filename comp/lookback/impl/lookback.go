// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package lookbackimpl implements the lookback component.
// This is currently a blackhole implementation that logs received samples;
// the WAL/ring-buffer storage will be added in a future iteration.
package lookbackimpl

import (
	"context"
	"sync/atomic"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	lookbackdef "github.com/DataDog/datadog-agent/comp/lookback/def"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// dependencies are the fx dependencies of the lookback component.
type dependencies struct {
	fx.In

	Lc     fx.Lifecycle
	Config config.Component
	Log    log.Component
}

// provides are the outputs of the lookback component.
type provides struct {
	fx.Out

	Comp lookbackdef.Component
}

// lookbackImpl is the blackhole implementation of the lookback component.
type lookbackImpl struct {
	config   config.Component
	log      log.Component
	received atomic.Int64
}

// NewComponent constructs and registers the lookback component.
func NewComponent(deps dependencies) (provides, error) {
	l := &lookbackImpl{config: deps.Config, log: deps.Log}
	deps.Lc.Append(fx.Hook{
		OnStart: l.start,
		OnStop:  l.stop,
	})
	return provides{
		Comp: l,
	}, nil
}

func (l *lookbackImpl) start(_ context.Context) error {
	interval := l.config.GetDuration("metric_lookback.interval")
	checks := l.config.GetStringSlice("metric_lookback.enabled_checks")
	l.log.Infof("lookback: started (blackhole mode) interval=%s checks=%v", interval, checks)
	return nil
}

func (l *lookbackImpl) stop(_ context.Context) error {
	l.log.Infof("lookback: stopped (received %d samples total)", l.received.Load())
	return nil
}

// RecordSample implements lookbackdef.Component.
func (l *lookbackImpl) RecordSample(_ checkid.ID, name string, value float64, _ []string, _ string, _ float64, _ metrics.MetricType) {
	n := l.received.Add(1)
	if n == 1 || n%10000 == 0 {
		l.log.Debugf("lookback: RecordSample #%d name=%s value=%f", n, name, value)
	}
}

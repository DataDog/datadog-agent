// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// ErrorsSampler is dedicated to catching traces containing spans with errors.
type ErrorsSampler struct {
	ScoreSampler
	RemotelyConfigured bool
}

// NewErrorsSampler returns an initialized Sampler dedicated to errors. It behaves
// just like the the normal ScoreEngine except for its GetType method (useful
// for reporting).
func NewErrorsSampler(conf *config.AgentConfig) *ErrorsSampler {
	s := newSampler(conf.ExtraSampleRate, conf.ErrorTPS, []string{"sampler:error"})
	return &ErrorsSampler{
		ScoreSampler:       ScoreSampler{Sampler: s, samplingRateKey: errorsRateKey, disabled: conf.ErrorTPS == 0},
		RemotelyConfigured: false,
	}
}

func (e *ErrorsSampler) UpdateTargetTPS(targetTPS float64, remotelyConfigured bool) {
	e.Sampler.updateTargetTPS(targetTPS)
	e.RemotelyConfigured = remotelyConfigured
}

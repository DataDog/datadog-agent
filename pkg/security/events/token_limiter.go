// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package events holds events related files
package events

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/seclwin/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// TokenLimiter limiter specific to anomaly detection
type TokenLimiter struct {
	getToken func(Event) string
	limiter  *utils.Limiter[string]
}

// Allow returns whether the event is allowed
func (tkl *TokenLimiter) Allow(event Event) bool {
	return tkl.limiter.Allow(tkl.getToken(event))
}

// SwapStats return dropped and allowed stats
func (tkl *TokenLimiter) SwapStats() []utils.LimiterStat {
	return tkl.limiter.SwapStats()
}

func (tkl *TokenLimiter) genGetTokenFnc(fields []eval.Field) error {
	var m model.Model
	event := m.NewEvent()

	for _, field := range fields {
		if _, err := event.GetFieldType(field); err != nil {
			return err
		}
	}

	tkl.getToken = func(event Event) string {
		var token string
		for i, field := range fields {
			value, err := event.GetFieldValue(field)
			if err != nil {
				return ""
			}

			if i == 0 {
				token = fmt.Sprintf("%s:%v", field, value)
			} else {
				token += fmt.Sprintf(";%s:%v", field, value)
			}
		}
		return token
	}

	return nil
}

// NewTokenLimiter returns a new rate limiter which is bucketed by fields
func NewTokenLimiter(maxUniqueToken int, numEventsAllowedPerPeriod int, period time.Duration, fields []eval.Field) (*TokenLimiter, error) {
	limiter, err := utils.NewLimiter[string](maxUniqueToken, numEventsAllowedPerPeriod, period)
	if err != nil {
		return nil, err
	}

	tkl := &TokenLimiter{
		limiter: limiter,
	}
	if err := tkl.genGetTokenFnc(fields); err != nil {
		return nil, err
	}

	return tkl, nil
}

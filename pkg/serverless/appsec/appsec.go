// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless
// +build serverless

// Package appsec provides a simple Application Security Monitoring API for
// serverless.
package appsec

import (
	"time"

	"github.com/DataDog/go-libddwaf"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type AppSec struct {
	cfg *Config
	// WAF handle instance of the appsec event rules.
	handle *waf.Handle
	// Events rate limiter to limit the max amount of appsec events we can send
	// per second.
	eventsRateLimiter *TokenTicker
}

// New returns a new AppSec instance if it is enabled with the DD_APPSEC_ENABLED
// env var. An error is returned when AppSec couldn't be started due to
// compilation or configuration errors. When AppSec is not enabled, the returned
// appsec instance is nil, along with a nil error (nil, nil return values).
func New() (*AppSec, error) {
	// Check if appsec is enabled
	if enabled, _, err := isEnabled(); err != nil {
		return nil, err
	} else if !enabled {
		log.Debug("appsec: security monitoring is not enabled: DD_SERVERLESS_APPSEC_ENABLED is not set to true")
		return nil, nil
	}

	// Check if AppSec can actually run properly
	if err := waf.Health(); err != nil {
		return nil, err
	}

	cfg, err := newConfig()
	if err != nil {
		return nil, err
	}

	handle, err := waf.NewHandle([]byte(staticRecommendedRule), cfg.obfuscator.KeyRegex, cfg.obfuscator.ValueRegex)
	if err != nil {
		return nil, err
	}

	eventsRateLimiter := NewTokenTicker(int64(cfg.traceRateLimit), int64(cfg.traceRateLimit))
	eventsRateLimiter.Start()

	return &AppSec{
		cfg:               cfg,
		handle:            handle,
		eventsRateLimiter: eventsRateLimiter,
	}, nil
}

// Close the AppSec instance.
func (a *AppSec) Close() error {
	a.handle.Close()
	a.eventsRateLimiter.Stop()
	return nil
}

// Monitor runs the security event rules and return the events as raw JSON byte
// array.
func (a *AppSec) Monitor(addresses map[string]interface{}) (events []byte) {
	log.Debugf("appsec: monitoring the request context %v", addresses)
	ctx := waf.NewContext(a.handle)
	if ctx == nil {
		return nil
	}
	defer ctx.Close()
	timeout := a.cfg.wafTimeout
	events, _, err := ctx.Run(addresses, timeout)
	if err != nil {
		if err == waf.ErrTimeout {
			log.Debugf("appsec: waf timeout value of %s reached", timeout)
		} else {
			log.Errorf("appsec: unexpected waf execution error: %v", err)
			return nil
		}
	}
	dt, _ := ctx.TotalRuntime()
	if len(events) > 0 {
		log.Debugf("appsec: security events found in %s: %s", time.Duration(dt), string(events))
	}
	if !a.eventsRateLimiter.Allow() {
		log.Debugf("appsec: security events discarded: the rate limit of %d events/s is reached", a.cfg.traceRateLimit)
		return nil
	}
	return events
}

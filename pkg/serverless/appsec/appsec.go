// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package appsec provides a simple Application Security Monitoring API for
// serverless.
package appsec

import (
	"encoding/json"
	"time"

	"github.com/DataDog/appsec-internal-go/appsec"
	"github.com/DataDog/datadog-agent/pkg/serverless/appsec/httpsec"
	"github.com/DataDog/datadog-agent/pkg/serverless/proxy"
	waf "github.com/DataDog/go-libddwaf"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func New() (*httpsec.ProxyLifecycleProcessor, error) {
	appsecInstance, err := newAppSec() // note that the assigned variable is in the parent scope
	if err != nil {
		return nil, err
	}
	if appsecInstance == nil {
		return nil, nil // appsec disabled
	}

	// AppSec monitors the invocations by acting as a proxy of the AWS Lambda Runtime API.
	lp := httpsec.NewProxyLifecycleProcessor(appsecInstance)
	proxy.Start(
		"127.0.0.1:9000",
		"127.0.0.1:9001",
		lp,
	)
	log.Debug("appsec: started successfully using the runtime api proxy monitoring mode")
	return lp, nil
}

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
func newAppSec() (*AppSec, error) {
	// Check if appsec is enabled
	if enabled, _, err := isEnabled(); err != nil {
		return nil, err
	} else if !enabled {
		log.Debug("appsec: security monitoring is not enabled: DD_SERVERLESS_APPSEC_ENABLED is not set to true")
		return nil, nil
	}

	// Check if AppSec can actually run properly
	if err := wafHealth(); err != nil {
		return nil, err
	}

	cfg, err := newConfig()
	if err != nil {
		return nil, err
	}

	var rules map[string]any
	if err := json.Unmarshal([]byte(appsec.StaticRecommendedRules), &rules); err != nil {
		return nil, err
	}
	handle, err := waf.NewHandle(rules, cfg.obfuscator.KeyRegex, cfg.obfuscator.ValueRegex)
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

// wafHealth is a simple test helper that returns the same thing as `waf.Health`
// used to return in `go-libddwaf` prior to v1.4.0
func wafHealth() error {
	if ok, err := waf.SupportsTarget(); !ok {
		return err
	}

	if ok, err := waf.Load(); !ok {
		return err
	}
	return nil
}

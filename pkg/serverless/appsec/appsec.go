// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package appsec provides a simple Application Security Monitoring API for
// serverless.
package appsec

import (
	appsecLog "github.com/DataDog/appsec-internal-go/log"
	waf "github.com/DataDog/go-libddwaf/v2"
	json "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/serverless/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/appsec/httpsec"
	"github.com/DataDog/datadog-agent/pkg/serverless/proxy"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//nolint:revive // TODO(ASM) Fix revive linter
func New(demux aggregator.Demultiplexer) (*httpsec.ProxyLifecycleProcessor, error) {
	appsecInstance, err := newAppSec() // note that the assigned variable is in the parent scope
	if err != nil {
		return nil, err
	}
	if appsecInstance == nil {
		return nil, nil // appsec disabled
	}

	// AppSec monitors the invocations by acting as a proxy of the AWS Lambda Runtime API.
	lp := httpsec.NewProxyLifecycleProcessor(appsecInstance, demux)
	proxy.Start(
		"127.0.0.1:9000",
		"127.0.0.1:9001",
		lp,
	)
	log.Debug("appsec: started successfully using the runtime api proxy monitoring mode")
	return lp, nil
}

//nolint:revive // TODO(ASM) Fix revive linter
type AppSec struct {
	cfg *config.Config
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
	if enabled, _, err := config.IsEnabled(); err != nil {
		return nil, err
	} else if !enabled {
		log.Debug("appsec: security monitoring is not enabled: DD_SERVERLESS_APPSEC_ENABLED is not set to true")
		return nil, nil
	}

	// Check if appsec is used as a standalone product (i.e with APM tracing)
	if config.IsStandalone() {
		log.Info("appsec: starting in standalone mode. APM tracing will be disabled for this service")
	}

	// Check if AppSec can actually run properly
	if err := wafHealth(); err != nil {
		return nil, err
	}

	cfg, err := config.NewConfig()
	if err != nil {
		return nil, err
	}

	var rules map[string]any
	if err := json.Unmarshal(cfg.Rules, &rules); err != nil {
		return nil, err
	}

	handle, err := waf.NewHandle(rules, cfg.Obfuscator.KeyRegex, cfg.Obfuscator.ValueRegex)
	if err != nil {
		return nil, err
	}

	eventsRateLimiter := NewTokenTicker(int64(cfg.TraceRateLimit), int64(cfg.TraceRateLimit))
	eventsRateLimiter.Start()

	return &AppSec{
		cfg:               cfg,
		handle:            handle,
		eventsRateLimiter: eventsRateLimiter,
	}, nil
}

// Close the AppSec instance.
func (a *AppSec) Close() error {
	panic("not called")
}

// Monitor runs the security event rules and return the events as a slice
// The monitored addresses are all persistent addresses
func (a *AppSec) Monitor(addresses map[string]any) *waf.Result {
	panic("not called")
}

// wafHealth is a simple test helper that returns the same thing as `waf.Health`
// used to return in `go-libddwaf` prior to v1.4.0
func wafHealth() error {
	panic("not called")
}

// canExtractSchemas checks that API Security is enabled
// and that sampling rate allows schema extraction for a specific monitoring instance
func (a *AppSec) canExtractSchemas() bool {
	panic("not called")
}

func init() {
	appsecLog.SetBackend(appsecLog.Backend{
		Trace:     log.Tracef,
		Debug:     log.Debugf,
		Info:      log.Infof,
		Errorf:    log.Errorf,
		Criticalf: log.Criticalf,
	})
}

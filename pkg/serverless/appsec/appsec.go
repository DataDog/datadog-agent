// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package appsec provides a simple Application Security Monitoring API for
// serverless.
package appsec

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"

	appsecLog "github.com/DataDog/appsec-internal-go/log"
	"github.com/DataDog/go-libddwaf/v4"
	"github.com/DataDog/go-libddwaf/v4/timer"
	"github.com/DataDog/go-libddwaf/v4/waferrors"
	json "github.com/json-iterator/go"

	"github.com/DataDog/appsec-internal-go/limiter"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/serverless/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/appsec/httpsec"
	"github.com/DataDog/datadog-agent/pkg/serverless/proxy"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//nolint:revive // TODO(ASM) Fix revive linter
func New(demux aggregator.Demultiplexer) (lp *httpsec.ProxyLifecycleProcessor, err error) {
	lp, _, err = NewWithShutdown(demux)
	return
}

// NewWithShutdown returns a new httpsec.ProxyLifecycleProcessor and a shutdown function that can be
// called to terminate the started proxy (releasing the bound port and closing the AppSec instance).
// This is mainly intended to be called in test code so that goroutines and ports are not leaked,
// but can be used in other code paths when it is useful to be able to perform a clean shut down.
func NewWithShutdown(demux aggregator.Demultiplexer) (lp *httpsec.ProxyLifecycleProcessor, shutdown func(context.Context) error, err error) {
	appsecInstance, err := newAppSec() // note that the assigned variable is in the parent scope
	if err != nil {
		log.Warnf("appsec: failed to start appsec: %v -- appsec features are not available!", err)
		// Nil-out the error so we don't incorrectly report it later on...
		err = nil
	} else if appsecInstance == nil {
		return nil, nil, nil // appsec disabled
	}

	lambdaRuntimeAPI := os.Getenv("AWS_LAMBDA_RUNTIME_API")
	if lambdaRuntimeAPI == "" {
		lambdaRuntimeAPI = "127.0.0.1:9001"
	}

	// AppSec monitors the invocations by acting as a proxy of the AWS Lambda Runtime API.
	lp = httpsec.NewProxyLifecycleProcessor(appsecInstance, demux)
	shutdownProxy := proxy.Start(
		"127.0.0.1:9000",
		lambdaRuntimeAPI,
		lp,
	)
	log.Debug("appsec: started successfully using the runtime api proxy monitoring mode")

	shutdown = func(ctx context.Context) error {
		err := shutdownProxy(ctx)
		if appsecInstance != nil {
			// Note: `errors.Join` discards any `nil` error it receives.
			err = errors.Join(err, appsecInstance.Close())
		}
		return err
	}

	return
}

//nolint:revive // TODO(ASM) Fix revive linter
type AppSec struct {
	cfg *config.Config
	// Diagnostics corresponding to the current ruleset loaded in the `handle`.
	rulesetDiagnostics libddwaf.Diagnostics
	// WAF handle instance of the appsec event rules.
	handle *libddwaf.Handle
	// Events rate limiter to limit the max amount of appsec events we can send
	// per second.
	eventsRateLimiter *limiter.TokenTicker
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

	builder, err := libddwaf.NewBuilder(cfg.Obfuscator.KeyRegex, cfg.Obfuscator.ValueRegex)
	if err != nil {
		return nil, fmt.Errorf("failed to create libddwaf.Builder: %w", err)
	}
	defer builder.Close()
	var diag libddwaf.Diagnostics
	if cfg.Rules != nil {
		var rules map[string]any
		if err := json.Unmarshal(cfg.Rules, &rules); err != nil {
			return nil, fmt.Errorf("failed to parse ruleset: %w", err)
		}
		diag, err = builder.AddOrUpdateConfig("configured", rules)
		if err != nil {
			return nil, fmt.Errorf("failed to add ruleset: %w", err)
		}
	} else {
		diag, err = builder.AddDefaultRecommendedRuleset()
		if err != nil {
			return nil, fmt.Errorf("failed to add default recommended ruleset: %w", err)
		}
	}
	// Report errors & warnings reported by the ruleset for posterity
	handleDiagnostics(diag)

	handle := builder.Build()

	eventsRateLimiter := limiter.NewTokenTicker(int64(cfg.TraceRateLimit), int64(cfg.TraceRateLimit))
	eventsRateLimiter.Start()

	return &AppSec{
		cfg:                cfg,
		rulesetDiagnostics: diag,
		handle:             handle,
		eventsRateLimiter:  eventsRateLimiter,
	}, nil
}

func handleDiagnostics(diag libddwaf.Diagnostics) {
	if diag.Version != "" {
		log.Debugf("appsec: loaded ruleset version %s", diag.Version)
	}
	diag.EachFeature(func(name string, feat *libddwaf.Feature) {
		if feat.Error != "" {
			log.Warnf("appsec: %s feature reported error: %s", name, feat.Error)
		}
		for msg, ids := range feat.Errors {
			log.Warnf("appsec: %s feature reported error: %q %s", name, ids, msg)
		}
		for msg, ids := range feat.Warnings {
			log.Warnf("appsec: %s feature reported warning: %q %s", name, ids, msg)
		}
	})
}

// Close the AppSec instance.
func (a *AppSec) Close() error {
	a.handle.Close()
	a.eventsRateLimiter.Stop()
	return nil
}

// Monitor runs the security event rules and return the events as a slice
// The monitored addresses are all persistent addresses
//
// This function always returns nil when an error occurs.
func (a *AppSec) Monitor(addresses map[string]any) *httpsec.MonitorResult {
	if a == nil {
		// This needs to be nil-safe so a nil [AppSec] instance is effectvely a no-op [httpsec.Monitorer].
		return nil
	}

	log.Debugf("appsec: monitoring the request context %v", addresses)
	const timerKey = "waf.duration_ext"
	ctx, err := a.handle.NewContext(timer.WithBudget(a.cfg.WafTimeout), timer.WithComponents(timerKey))
	if err != nil {
		log.Errorf("appsec: failed to create waf context: %v", err)
		return nil
	}
	if ctx == nil {
		return nil
	}
	defer ctx.Close()

	// Ask the WAF for schema reporting if API security is enabled
	if a.canExtractSchemas() {
		addresses["waf.context.processor"] = map[string]any{"extract-schema": true}
	}

	timeout := false
	res, err := ctx.Run(libddwaf.RunAddressData{TimerKey: timerKey, Persistent: addresses})
	if err != nil {
		if err == waferrors.ErrTimeout {
			log.Debugf("appsec: waf timeout value of %s reached", a.cfg.WafTimeout)
			timeout = true
		} else {
			log.Errorf("appsec: unexpected waf execution error: %v", err)
			return nil
		}
	}

	stats := ctx.Timer.Stats()
	if res.HasEvents() {
		log.Debugf("appsec: security events found in %s: %v", stats[timerKey], res.Events)
	}
	if !a.eventsRateLimiter.Allow() {
		log.Debugf("appsec: security events discarded: the rate limit of %d events/s is reached", a.cfg.TraceRateLimit)
		return nil
	}

	return &httpsec.MonitorResult{
		Result:      res,
		Diagnostics: a.rulesetDiagnostics,
		Timeout:     timeout,
		Timings:     stats,
	}
}

// wafHealth is a simple test helper that returns the same thing as `waf.Health`
// used to return in `go-libddwaf` prior to v1.4.0
func wafHealth() error {
	if ok, err := libddwaf.Usable(); !ok {
		return err
	}

	if ok, err := libddwaf.Load(); !ok {
		return err
	}
	return nil
}

// canExtractSchemas checks that API Security is enabled
// and that sampling rate allows schema extraction for a specific monitoring instance
func (a *AppSec) canExtractSchemas() bool {
	return a.cfg.APISec.Enabled &&
		//nolint:staticcheck // SA1019 we will migrate to the new APISec sampler at a later point.
		a.cfg.APISec.SampleRate >=
			rand.Float64()
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

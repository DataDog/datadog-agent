// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package appsec provides a simple Application Security Monitoring API for
// serverless.
package appsec

import (
	"github.com/DataDog/datadog-agent/pkg/serverless/appsec/waf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type AppSec struct {
	cfg *Config
	// WAF handle instance of the appsec event rules.
	handle *waf.Handle
}

// Context of security monitoring execution. Usually one per request to monitor.
type Context struct {
	// Pointer to the AppSec instance containing the WAF rules instance and the
	// AppSec configuration.
	instance *AppSec
	// Map of security rules' addresses and their values.
	addresses map[string]interface{}
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
	return &AppSec{
		cfg:    cfg,
		handle: handle,
	}, nil
}

// Close the AppSec instance. The underlying WAF instance is free'd only when
// no more
func (a *AppSec) Close() error {
	a.handle.Close()
	return nil
}

// Monitor runs the security event rules and return the events as raw JSON byte
// array.
func (a *AppSec) Monitor(addresses map[string]interface{}) (events []byte) {
	ctx := waf.NewContext(a.handle)
	if ctx == nil {
		return
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
	log.Debug("appsec: waf execution run time: ", dt)
	return events
}

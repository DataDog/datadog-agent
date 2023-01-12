// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless
// +build !serverless

// Package appsec provides a simple Application Security Monitoring API for
// serverless.
package appsec

type AppSec struct{}

// New returns a new AppSec instance if it is enabled with the DD_APPSEC_ENABLED
// env var. An error is returned when AppSec couldn't be started due to
// compilation or configuration errors. When AppSec is not enabled, the returned
// appsec instance is nil, along with a nil error (nil, nil return values).
func New() (*AppSec, error) { return nil, nil }

// Close the AppSec instance.
func (a *AppSec) Close() error { return nil }

// Monitor runs the security event rules and return the events as raw JSON byte
// array.
func (a *AppSec) Monitor(addresses map[string]interface{}) (events []byte) { return nil }

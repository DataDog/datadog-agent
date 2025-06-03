// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package apminject implements the apm injector installer
package apminject

type ssiStatusProviderImpl struct{}

// Provider defines the interface to retrieve the status of the APM auto-instrumentation
type Provider interface {
	AutoInstrumentationStatus() (bool, []string, error)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package check

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// StubCheck stubs a check, should only be used in tests
type StubCheck struct{}

func (c *StubCheck) String() string                                             { return "StubCheck" }
func (c *StubCheck) Version() string                                            { return "" }
func (c *StubCheck) ConfigSource() string                                       { return "" }
func (c *StubCheck) Stop()                                                      {}
func (c *StubCheck) Configure(integration.Data, integration.Data, string) error { return nil }
func (c *StubCheck) Interval() time.Duration                                    { return 1 * time.Second }
func (c *StubCheck) Run() error                                                 { return nil }
func (c *StubCheck) ID() ID                                                     { return ID(c.String()) }
func (c *StubCheck) GetWarnings() []error                                       { return []error{} }
func (c *StubCheck) GetMetricStats() (map[string]int64, error)                  { return make(map[string]int64), nil }
func (c *StubCheck) IsTelemetryEnabled() bool                                   { return false }

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package corechecks

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// FIXTURE
type TestCheck struct{}

func (c *TestCheck) String() string                            { return "TestCheck" }
func (c *TestCheck) Version() string                           { return "" }
func (c *TestCheck) ConfigSource() string                      { return "" }
func (c *TestCheck) Run() error                                { return nil }
func (c *TestCheck) Stop()                                     {}
func (c *TestCheck) Interval() time.Duration                   { return 1 }
func (c *TestCheck) ID() check.ID                              { return check.ID(c.String()) }
func (c *TestCheck) GetWarnings() []error                      { return []error{} }
func (c *TestCheck) GetMetricStats() (map[string]int64, error) { return make(map[string]int64), nil }
func (c *TestCheck) IsTelemetryEnabled() bool                  { return false }
func (c *TestCheck) Configure(data integration.Data, initData integration.Data, source string) error {
	if string(data) == "err" {
		return fmt.Errorf("testError")
	}
	return nil
}

func TestNewGoCheckLoader(t *testing.T) {
	if checkLoader, _ := NewGoCheckLoader(); checkLoader == nil {
		t.Fatal("Expected loader instance, found: nil")
	}
}

func testCheckFactory() check.Check {
	return &TestCheck{}
}

func TestRegisterCheck(t *testing.T) {
	RegisterCheck("foo", testCheckFactory)
	_, found := catalog["foo"]
	if !found {
		t.Fatal("Check foo not found in catalog")
	}
}

func TestLoad(t *testing.T) {
	RegisterCheck("foo", testCheckFactory)

	// check is in catalog, pass 1 good instance
	i := []integration.Data{
		integration.Data("foo: bar"),
	}
	cc := integration.Config{Name: "foo", Instances: i}
	l, _ := NewGoCheckLoader()

	_, err := l.Load(cc, i[0])

	if err != nil {
		t.Fatalf("Expected nil error, found: %v", err)
	}

	// check is in catalog, pass 1 bad instance
	i = []integration.Data{
		integration.Data("err"),
	}
	cc = integration.Config{Name: "foo", Instances: i}

	_, err = l.Load(cc, i[0])

	if err == nil {
		t.Fatalf("Expected error, found: nil")
	}

	// check not in catalog
	i = []integration.Data{
		integration.Data("foo: bar"),
	}
	cc = integration.Config{Name: "bar", Instances: i}

	_, err = l.Load(cc, i[0])

	if err == nil {
		t.Fatal("Expected error, found: nil")
	}
}

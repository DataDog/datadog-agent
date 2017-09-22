// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package corechecks

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// FIXTURE
type TestCheck struct{}

func (c *TestCheck) String() string                                     { return "TestCheck" }
func (c *TestCheck) Configure(check.ConfigData, check.ConfigData) error { return nil }
func (c *TestCheck) Run() error                                         { return nil }
func (c *TestCheck) Stop()                                              {}
func (c *TestCheck) Interval() time.Duration                            { return 1 }
func (c *TestCheck) ID() check.ID                                       { return check.ID(c.String()) }
func (c *TestCheck) GetWarnings() []error                               { return []error{} }
func (c *TestCheck) GetMetricStats() (map[string]int64, error)          { return make(map[string]int64), nil }

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

	// check is in catalog, pass 2 instances
	i := []check.ConfigData{
		check.ConfigData("foo: bar"),
		check.ConfigData("bar: baz"),
	}
	cc := check.Config{Name: "foo", Instances: i}
	l, _ := NewGoCheckLoader()

	lst, err := l.Load(cc)

	if err != nil {
		t.Fatalf("Expected nil error, found: %v", err)
	}
	if len(lst) != 2 {
		t.Fatalf("Expected 2 checks, found: %d", len(lst))
	}

	// check is in catalog, pass no instances
	i = []check.ConfigData{}
	cc = check.Config{Name: "foo", Instances: i}

	lst, err = l.Load(cc)

	if err != nil {
		t.Fatalf("Expected nil error, found: %v", err)
	}
	if len(lst) != 0 {
		t.Fatalf("Expected 0 checks, found: %d", len(lst))
	}

	// check not in catalog
	cc = check.Config{Name: "bar", Instances: nil}

	lst, err = l.Load(cc)

	if err == nil {
		t.Fatal("Expected error, found: nil")
	}
	if len(lst) != 0 {
		t.Fatalf("Expected 0 checks, found: %d", len(lst))
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package corechecks

import (
	"errors"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// FIXTURE
type TestCheck struct {
	stub.StubCheck
}

//nolint:revive // TODO(AML) Fix revive linter
func (c *TestCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initData integration.Data, source string) error {
	if string(data) == "err" {
		return fmt.Errorf("testError")
	}
	if string(data) == "skip" {
		return check.ErrSkipCheckInstance
	}
	return nil
}

func TestNewGoCheckLoader(t *testing.T) {
	if checkLoader, _ := NewGoCheckLoader(); checkLoader == nil {
		t.Fatal("Expected loader instance, found: nil")
	}
}

func testCheckNew() optional.Option[func() check.Check] {
	return optional.NewOption(func() check.Check {
		return &TestCheck{}
	})
}

func TestRegisterCheck(t *testing.T) {
	RegisterCheck("foo", testCheckNew())
	_, found := catalog["foo"]
	if !found {
		t.Fatal("Check foo not found in catalog")
	}
}

func TestLoad(t *testing.T) {
	RegisterCheck("foo", testCheckNew())

	// check is in catalog, pass 1 good instance
	i := []integration.Data{
		integration.Data("foo: bar"),
	}
	cc := integration.Config{Name: "foo", Instances: i}
	l, _ := NewGoCheckLoader()

	_, err := l.Load(aggregator.NewNoOpSenderManager(), cc, i[0])
	if err != nil {
		t.Fatalf("Expected nil error, found: %v", err)
	}

	// check is in catalog, pass 1 bad instance
	i = []integration.Data{
		integration.Data("err"),
	}
	cc = integration.Config{Name: "foo", Instances: i}

	_, err = l.Load(aggregator.NewNoOpSenderManager(), cc, i[0])

	if err == nil {
		t.Fatalf("Expected error, found: nil")
	}

	// check is in catalog, pass 1 skip instance
	i = []integration.Data{
		integration.Data("skip"),
	}
	cc = integration.Config{Name: "foo", Instances: i}

	_, err = l.Load(aggregator.NewNoOpSenderManager(), cc, i[0])

	if !errors.Is(err, check.ErrSkipCheckInstance) {
		t.Fatalf("Expected ErrSkipCheckInstance, found: %v", err)
	}

	// check not in catalog
	i = []integration.Data{
		integration.Data("foo: bar"),
	}
	cc = integration.Config{Name: "bar", Instances: i}

	_, err = l.Load(aggregator.NewNoOpSenderManager(), cc, i[0])

	if err == nil {
		t.Fatal("Expected error, found: nil")
	}
}

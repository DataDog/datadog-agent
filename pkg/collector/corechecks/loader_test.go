// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package corechecks

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// FIXTURE
type TestCheck struct {
	stub.StubCheck
}

func (c *TestCheck) Configure(_ sender.SenderManager, _ uint64, data integration.Data, _ integration.Data, _ string, _ string) error {
	if string(data) == "err" {
		return errors.New("testError")
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

func testCheckNew() option.Option[func() check.Check] {
	return option.New(func() check.Check {
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

func TestRegisterCheckWithLoaderSupport(t *testing.T) {
	RegisterCheckWithLoaderSupport("foo_with_support", testCheckNew(), func(integration.Config, integration.Data) check.LoaderSupport {
		return check.LoaderSupportUnsupported
	})
	entry, found := catalog["foo_with_support"]
	if !found {
		t.Fatal("Check foo_with_support not found in catalog")
	}
	if entry.loaderSupport == nil {
		t.Fatal("Check foo_with_support has no loader support predicate")
	}
}

func TestSupportsConfigUsesCatalogWithoutConstructingCheck(t *testing.T) {
	factoryCalls := 0
	checkName := "metadata_only"
	RegisterCheck(checkName, option.New(func() check.Check {
		factoryCalls++
		return &TestCheck{}
	}))
	l, _ := NewGoCheckLoader()

	support := l.SupportsConfig(integration.Config{Name: checkName}, integration.Data("{}"))

	if support != check.LoaderSupportSupported {
		t.Fatalf("Expected supported, found: %v", support)
	}
	if factoryCalls != 0 {
		t.Fatalf("Expected no factory calls, found: %d", factoryCalls)
	}

	support = l.SupportsConfig(integration.Config{Name: "missing_metadata_only"}, integration.Data("{}"))
	if support != check.LoaderSupportUnsupported {
		t.Fatalf("Expected unsupported, found: %v", support)
	}
}

func TestSupportsConfigUsesRegisteredSupportWithoutConstructingCheck(t *testing.T) {
	factoryCalls := 0
	supportCalls := 0
	checkName := "metadata_with_support"
	RegisterCheckWithLoaderSupport(checkName, option.New(func() check.Check {
		factoryCalls++
		return &TestCheck{}
	}), func(config integration.Config, instance integration.Data) check.LoaderSupport {
		supportCalls++
		if config.Name != checkName {
			t.Fatalf("Expected config name %q, found: %q", checkName, config.Name)
		}
		if string(instance) != "{}" {
			t.Fatalf("Expected instance {}, found: %q", string(instance))
		}
		return check.LoaderSupportUnsupported
	})
	l, _ := NewGoCheckLoader()

	support := l.SupportsConfig(integration.Config{Name: checkName}, integration.Data("{}"))

	if support != check.LoaderSupportUnsupported {
		t.Fatalf("Expected unsupported, found: %v", support)
	}
	if supportCalls != 1 {
		t.Fatalf("Expected one support call, found: %d", supportCalls)
	}
	if factoryCalls != 0 {
		t.Fatalf("Expected no factory calls, found: %d", factoryCalls)
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

	_, err := l.Load(aggregator.NewNoOpSenderManager(), cc, i[0], 0)
	if err != nil {
		t.Fatalf("Expected nil error, found: %v", err)
	}

	// check is in catalog, pass 1 bad instance
	i = []integration.Data{
		integration.Data("err"),
	}
	cc = integration.Config{Name: "foo", Instances: i}

	_, err = l.Load(aggregator.NewNoOpSenderManager(), cc, i[0], 0)

	if err == nil {
		t.Fatalf("Expected error, found: nil")
	}

	// check is in catalog, pass 1 skip instance
	i = []integration.Data{
		integration.Data("skip"),
	}
	cc = integration.Config{Name: "foo", Instances: i}

	_, err = l.Load(aggregator.NewNoOpSenderManager(), cc, i[0], 0)

	if !errors.Is(err, check.ErrSkipCheckInstance) {
		t.Fatalf("Expected ErrSkipCheckInstance, found: %v", err)
	}

	// check not in catalog
	i = []integration.Data{
		integration.Data("foo: bar"),
	}
	cc = integration.Config{Name: "bar", Instances: i}

	_, err = l.Load(aggregator.NewNoOpSenderManager(), cc, i[0], 0)

	if err == nil {
		t.Fatal("Expected error, found: nil")
	}
}

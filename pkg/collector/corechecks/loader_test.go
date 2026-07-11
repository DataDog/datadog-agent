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
	configured     bool
	configDigest   uint64
	configData     integration.Data
	configSource   string
	configProvider string
}

func (c *TestCheck) Configure(_ sender.SenderManager, digest uint64, data integration.Data, _ integration.Data, source string, provider string) error {
	c.configured = true
	c.configDigest = digest
	c.configData = data
	c.configSource = source
	c.configProvider = provider
	if string(data) == "err" {
		return errors.New("testError")
	}
	if string(data) == "skip" {
		return check.ErrSkipCheckInstance
	}
	return nil
}

func withTestCatalog(t *testing.T) {
	t.Helper()
	WithTestCatalog(t)
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
	withTestCatalog(t)
	RegisterCheck("foo", testCheckNew())
	_, found := catalog["foo"]
	if !found {
		t.Fatal("Check foo not found in catalog")
	}
}

func TestGoCheckLoaderPassesDefaultNormalMode(t *testing.T) {
	withTestCatalog(t)

	var gotMode LoadMode
	RegisterContextualCheck("foo", option.New(func(ctx ConstructionContext) check.Check {
		gotMode = ctx.Mode
		return &TestCheck{}
	}))

	l, _ := NewGoCheckLoader()
	_, err := l.Load(aggregator.NewNoOpSenderManager(), integration.Config{Name: "foo"}, integration.Data("foo: bar"), 0)
	if err != nil {
		t.Fatalf("Expected nil error, found: %v", err)
	}
	if gotMode != NormalLoadMode {
		t.Fatalf("Expected mode %q, found %q", NormalLoadMode, gotMode)
	}
}

func TestGoCheckLoaderPassesShadowMode(t *testing.T) {
	withTestCatalog(t)

	var gotMode LoadMode
	RegisterContextualCheck("foo", option.New(func(ctx ConstructionContext) check.Check {
		gotMode = ctx.Mode
		return &TestCheck{}
	}))

	l, _ := NewGoCheckLoader(WithLoadMode(ShadowLoadMode))
	_, err := l.Load(aggregator.NewNoOpSenderManager(), integration.Config{Name: "foo"}, integration.Data("foo: bar"), 0)
	if err != nil {
		t.Fatalf("Expected nil error, found: %v", err)
	}
	if gotMode != ShadowLoadMode {
		t.Fatalf("Expected mode %q, found %q", ShadowLoadMode, gotMode)
	}
}

func TestRegisterCheckWrapsLegacyFactory(t *testing.T) {
	withTestCatalog(t)

	called := false
	RegisterCheck("foo", option.New(func() check.Check {
		called = true
		return &TestCheck{}
	}))

	l, _ := NewGoCheckLoader(WithLoadMode(ShadowLoadMode))
	_, err := l.Load(aggregator.NewNoOpSenderManager(), integration.Config{Name: "foo"}, integration.Data("foo: bar"), 0)
	if err != nil {
		t.Fatalf("Expected nil error, found: %v", err)
	}
	if !called {
		t.Fatal("Expected legacy factory to be called")
	}
}

func TestLoad(t *testing.T) {
	withTestCatalog(t)
	RegisterCheck("foo", testCheckNew())

	// check is in catalog, pass 1 good instance
	i := []integration.Data{
		integration.Data("foo: bar"),
	}
	cc := integration.Config{Name: "foo", Instances: i, Source: "file:foo.yaml", Provider: "file"}
	l, _ := NewGoCheckLoader()

	loadedCheck, err := l.Load(aggregator.NewNoOpSenderManager(), cc, i[0], 0)
	if err != nil {
		t.Fatalf("Expected nil error, found: %v", err)
	}
	tc := loadedCheck.(*TestCheck)
	if !tc.configured {
		t.Fatal("Expected check to be configured")
	}
	if tc.configDigest != cc.FastDigest() {
		t.Fatalf("Expected digest %d, found %d", cc.FastDigest(), tc.configDigest)
	}
	if string(tc.configData) != string(i[0]) {
		t.Fatalf("Expected instance data %q, found %q", i[0], tc.configData)
	}
	if tc.configSource != "file:foo.yaml[0]" {
		t.Fatalf("Expected source %q, found %q", "file:foo.yaml[0]", tc.configSource)
	}
	if tc.configProvider != "file" {
		t.Fatalf("Expected provider %q, found %q", "file", tc.configProvider)
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

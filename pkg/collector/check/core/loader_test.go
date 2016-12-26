package core

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// FIXTURE
type TestCheck struct{}

func (c *TestCheck) String() string                   { return "TestCheck" }
func (c *TestCheck) Configure(check.ConfigData) error { return nil }
func (c *TestCheck) InitSender()                      {}
func (c *TestCheck) Run() error                       { return nil }
func (c *TestCheck) Stop()                            {}
func (c *TestCheck) Interval() time.Duration          { return 1 }
func (c *TestCheck) ID() string                       { return c.String() }

func TestNewGoCheckLoader(t *testing.T) {
	if NewGoCheckLoader() == nil {
		t.Fatal("Expected loader instance, found: nil")
	}
}

func TestRegisterCheck(t *testing.T) {
	RegisterCheck("foo", new(TestCheck))
	_, found := catalog["foo"]
	if !found {
		t.Fatal("Check foo not found in catalog")
	}
}

func TestLoad(t *testing.T) {
	RegisterCheck("foo", new(TestCheck))

	// check is in catalog, pass 2 instances
	i := []check.ConfigData{
		check.ConfigData("foo: bar"),
		check.ConfigData("bar: baz"),
	}
	cc := check.Config{Name: "foo", Instances: i}
	l := NewGoCheckLoader()

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

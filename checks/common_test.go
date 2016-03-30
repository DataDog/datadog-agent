package checks

import (
	"errors"
	"testing"
)

// FIXTURE
type TestCheck struct {
	doErr bool
}

func (c *TestCheck) String() string { return "TestCheck" }

func (c *TestCheck) Run() (CheckResult, error) {
	if c.doErr {
		msg := "A tremendous error occurred."
		return CheckResult{Result: "", Error: msg}, errors.New(msg)
	}
	return CheckResult{Result: "Foo", Error: ""}, nil
}

func TestRunner(t *testing.T) {
	pending := make(chan Check)
	results := make(chan CheckResult)
	go Runner(pending, results)

	pending <- &TestCheck{doErr: false}
	res := <-results
	if res.Error != "" {
		t.Fatalf("Expected empty error message, found: %s", res.Error)
	}
	if res.Result != "Foo" {
		t.Fatalf("Expected: %s, found: %s", "Foo", res.Result)
	}

	pending <- &TestCheck{doErr: true}
	res = <-results
	if res.Error == "" {
		t.Fatal("Found empty error message")
	}
	if res.Result != "" {
		t.Fatalf("Expected empty Result")
	}
}

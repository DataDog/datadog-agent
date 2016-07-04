package checks

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/check"
)

// FIXTURE
type TestCheck struct {
	doErr bool
}

func (c *TestCheck) String() string { return "TestCheck" }

func (c *TestCheck) Configure(check.ConfigData) {}

func (c *TestCheck) Run() error {
	if c.doErr {
		msg := "A tremendous error occurred."
		return errors.New(msg)
	}
	return nil
}

func TestRunner(t *testing.T) {
	pending := make(chan check.Check)
	go check.Runner(pending)

	pending <- &TestCheck{doErr: false}
	pending <- &TestCheck{doErr: true}
}

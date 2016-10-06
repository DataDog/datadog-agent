package check

import (
	"errors"
	"testing"
	"time"
)

// FIXTURE
type TestCheck struct {
	doErr bool
}

func (c *TestCheck) String() string          { return "TestCheck" }
func (c *TestCheck) Configure(ConfigData)    {}
func (c *TestCheck) Interval() time.Duration { return 1 }
func (c *TestCheck) Run() error {
	if c.doErr {
		msg := "A tremendous error occurred."
		return errors.New(msg)
	}
	return nil
}

func TestRunner(t *testing.T) {
	pending := make(chan Check)
	go Runner(pending)

	pending <- &TestCheck{doErr: false}
	pending <- &TestCheck{doErr: true}
}

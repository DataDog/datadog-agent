package check

import (
	"errors"
	"time"
)

// FIXTURE
type TestCheck struct {
	doErr  bool
	hasRun bool
}

func (c *TestCheck) String() string             { return "TestCheck" }
func (c *TestCheck) Stop()                      {}
func (c *TestCheck) Configure(ConfigData) error { return nil }
func (c *TestCheck) InitSender()                {}
func (c *TestCheck) Interval() time.Duration    { return 1 }
func (c *TestCheck) Run() error {
	if c.doErr {
		msg := "A tremendous error occurred."
		return errors.New(msg)
	}
	c.hasRun = true
	return nil
}
func (c *TestCheck) ID() string { return c.String() }

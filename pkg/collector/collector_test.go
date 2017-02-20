package collector

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
)

// FIXTURE
type TestCheck struct {
	stop chan bool
}

func (c *TestCheck) String() string                        { return "TestCheck" }
func (c *TestCheck) Stop()                                 { c.stop <- true }
func (c *TestCheck) Configure(a, b check.ConfigData) error { return nil }
func (c *TestCheck) InitSender()                           {}
func (c *TestCheck) Interval() time.Duration               { return 1 }
func (c *TestCheck) Run() error                            { <-c.stop; return nil }
func (c *TestCheck) ID() check.ID                          { return check.ID(c.String()) }

func NewCheck() *TestCheck { return &TestCheck{stop: make(chan bool)} }

func TestNewCollector(t *testing.T) {
	c := NewCollector()
	assert.Nil(t, c.aggregator)
	assert.Nil(t, c.runner)
	assert.Nil(t, c.scheduler)
	assert.Nil(t, c.pyState)
	assert.Equal(t, stopped, c.state)
}

func TestStartStop(t *testing.T) {
	c := NewCollector()
	c.Start(".")
	assert.NotNil(t, c.aggregator)
	assert.NotNil(t, c.runner)
	assert.NotNil(t, c.scheduler)
	assert.NotNil(t, c.pyState)
	assert.Equal(t, started, c.state)
	c.Stop()
	assert.Nil(t, c.aggregator)
	assert.Nil(t, c.runner)
	assert.Nil(t, c.scheduler)
	assert.Nil(t, c.pyState)
	assert.Equal(t, stopped, c.state)
}

func TestRunCheck(t *testing.T) {
	ch := NewCheck()
	c := NewCollector()

	// collector hasn't started
	err := c.RunCheck(ch)
	assert.NotNil(t, err)
	assert.Equal(t, "the collector is not running", err.Error())

	// start the collector
	c.Start(".")

	// schedule a check
	err = c.RunCheck(ch)
	assert.Equal(t, 1, len(c.checks))
	assert.Equal(t, ch, c.checks["TestCheck"])

	// schedule the same check twice
	err = c.RunCheck(ch)
	assert.NotNil(t, err)
	assert.Equal(t, "a check with ID TestCheck is already running", err.Error())

	c.Stop()
}

func TestReloadCheck(t *testing.T) {
	ch := NewCheck()
	c := NewCollector()

	// collector hasn't started
	empty := check.ConfigData{}
	err := c.ReloadCheck("foo", empty, empty)
	assert.NotNil(t, err)
	assert.Equal(t, "the collector is not running", err.Error())

	// start the collector and schedule a check
	c.Start(".")
	err = c.RunCheck(ch)

	// check doesn't exist
	err = c.ReloadCheck("foo", empty, empty)
	assert.NotNil(t, err)
	assert.Equal(t, "cannot find a check with ID foo", err.Error())

	// all good
	err = c.ReloadCheck("TestCheck", empty, empty)
	assert.Nil(t, err)

	c.Stop()
}

func TestStopCheck(t *testing.T) {
	ch := NewCheck()
	c := NewCollector()

	// collector hasn't started
	empty := check.ConfigData{}
	err := c.ReloadCheck("foo", empty, empty)
	assert.NotNil(t, err)
	assert.Equal(t, "the collector is not running", err.Error())

	// start the collector and schedule a check
	c.Start(".")
	err = c.RunCheck(ch)

	// all good
	err = c.StopCheck("TestCheck")
	assert.Nil(t, err)
	assert.Zero(t, len(c.checks))

	c.Stop()
}

func TestFind(t *testing.T) {
	c := NewCollector()
	assert.False(t, c.find("bar"))

	c.checks["bar"] = nil
	assert.False(t, c.find("foo"))
	assert.True(t, c.find("bar"))
}

func TestDelete(t *testing.T) {
	c := NewCollector()
	c.checks["bar"] = nil
	// delete a key that doesn't exist should be a noop
	c.delete("foo")

	// for good
	assert.True(t, c.find("bar"))
	c.delete("bar")
	assert.False(t, c.find("bar"))
}

func TestStarted(t *testing.T) {
	c := NewCollector()
	assert.False(t, c.started())
	c.Start(".")
	assert.True(t, c.started())
	c.Stop()
}

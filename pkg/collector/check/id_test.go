package check

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// FIXTURE
type TestCheck struct{}

func (c *TestCheck) String() string                         { return "TestCheck" }
func (c *TestCheck) Stop()                                  {}
func (c *TestCheck) Configure(ConfigData, ConfigData) error { return nil }
func (c *TestCheck) InitSender()                            {}
func (c *TestCheck) Interval() time.Duration                { return 1 }
func (c *TestCheck) Run() error                             { return nil }
func (c *TestCheck) ID() ID                                 { return ID(c.String()) }

func TestIdentify(t *testing.T) {
	testCheck := &TestCheck{}

	instance1 := ConfigData("key1:value1\nkey2:value2")
	initConfig1 := ConfigData("key:value")
	instance2 := instance1
	initConfig2 := initConfig1
	assert.Equal(t, Identify(testCheck, instance1, initConfig1), Identify(testCheck, instance2, initConfig2))

	instance3 := ConfigData("key1:value1\nkey2:value3")
	initConfig3 := ConfigData("key:value")
	assert.NotEqual(t, Identify(testCheck, instance1, initConfig1), Identify(testCheck, instance3, initConfig3))
}

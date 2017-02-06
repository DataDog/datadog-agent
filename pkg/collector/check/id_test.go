package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

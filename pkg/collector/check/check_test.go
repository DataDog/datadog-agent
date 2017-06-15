package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigEqual(t *testing.T) {
	config := &Config{}
	assert.False(t, config.Equal(nil))

	another := &Config{}
	assert.True(t, config.Equal(another))

	another.Name = "foo"
	assert.False(t, config.Equal(another))
	config.Name = another.Name
	assert.True(t, config.Equal(another))

	another.InitConfig = ConfigData("fooBarBaz")
	assert.False(t, config.Equal(another))
	config.InitConfig = another.InitConfig
	assert.True(t, config.Equal(another))

	another.Instances = []ConfigData{ConfigData("justFoo")}
	assert.False(t, config.Equal(another))
	config.Instances = another.Instances
	assert.True(t, config.Equal(another))
}

func TestString(t *testing.T) {
	config := &Config{}
	assert.False(t, config.Equal(nil))

	config.Name = "foo"
	config.InitConfig = ConfigData("fooBarBaz")
	config.Instances = []ConfigData{ConfigData("justFoo")}

	expected := `init_config:
- fooBarBaz
instances:
- justFoo
`
	assert.Equal(t, config.String(), expected)
}

// NOTICE: See TestMain function in `utils_test.go` for Python initialization
package py

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	l := NewPythonCheckLoader()
	config := check.Config{Name: "testcheck"}
	config.Instances = append(config.Instances, []byte("foo: bar"))
	config.Instances = append(config.Instances, []byte("bar: baz"))

	instances, err := l.Load(config)
	assert.Nil(t, err)
	assert.Equal(t, len(instances), 2)

	// the python module doesn't exist
	config = check.Config{Name: "doesntexist"}
	instances, err = l.Load(config)
	assert.NotNil(t, err)
	assert.Zero(t, len(instances))

	// the python module contains errors
	config = check.Config{Name: "bad"}
	instances, err = l.Load(config)
	assert.NotNil(t, err)
	assert.Zero(t, len(instances))

	// the python module is good but nothing derives from AgentCheck
	config = check.Config{Name: "foo"}
	instances, err = l.Load(config)
	assert.NotNil(t, err)
	assert.Zero(t, len(instances))
}

func TestNewPythonCheckLoader(t *testing.T) {
	loader := NewPythonCheckLoader()
	assert.NotNil(t, loader)
}

package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewProvider(t *testing.T) {
	p := NewFileProvider("foo")
	assert.Equal(t, p.searchPath, "foo")
}

func TestConfigure(t *testing.T) {
	c := NewConfig()
	p := NewFileProvider("foo")

	err := p.Configure(c)
	assert.NotNil(t, err)
	assert.EqualError(t, err, "open foo/datadog.conf: no such file or directory")

	p.searchPath = filepath.Join("test", "failing")
	err = p.Configure(c)
	assert.NotNil(t, err)
	assert.EqualError(t, err, "yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `not a y...` into config.Config")

	p.searchPath = "test"
	err = p.Configure(c)
	assert.Nil(t, err)
	assert.Equal(t, c.DdURL, "https://foo.datadoghq.com")
}

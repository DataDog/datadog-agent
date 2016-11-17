package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewProvider(t *testing.T) {
	p := NewFileProvider([]string{"foo"})
	assert.Equal(t, p.searchPaths[0], "foo")
}

func TestConfigure(t *testing.T) {
	c := NewConfig()
	p := NewFileProvider([]string{"foo"})

	err := p.Configure(c)
	assert.NotNil(t, err)
	assert.EqualError(t, err, "Unable to find a valid config file in any of the paths: [foo]")

	p.searchPaths = []string{filepath.Join("test", "failing")}
	err = p.Configure(c)
	assert.NotNil(t, err)
	assert.EqualError(t, err, "yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `not a y...` into config.Config")

	p.searchPaths = []string{"test"}
	err = p.Configure(c)
	assert.Nil(t, err)
	assert.Equal(t, c.DdURL, "https://foo.datadoghq.com")
}

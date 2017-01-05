package loader

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetCheckConfig(t *testing.T) {
	// file does not exist
	config, err := getCheckConfig("foo", "")
	assert.NotNil(t, err)

	// file contains invalid Yaml
	config, err = getCheckConfig("foo", "tests/invalid.yaml")
	assert.NotNil(t, err)

	// valid yaml, invalid configuration file
	config, err = getCheckConfig("foo", "tests/notaconfig.yaml")
	assert.NotNil(t, err)
	assert.Equal(t, len(config.Instances), 0)

	// valid configuration file
	config, err = getCheckConfig("foo", "tests/testcheck.yaml")
	assert.Nil(t, err)
	assert.Equal(t, config.Name, "foo")

	assert.Equal(t, []byte(config.InitConfig), []byte("- test: 21\n"))
	assert.Equal(t, len(config.Instances), 1)
	assert.Equal(t, []byte(config.Instances[0]), []byte("foo: bar\n"))
}

func TestNewYamlConfigProvider(t *testing.T) {
	paths := []string{"foo", "bar", "foo/bar"}
	provider := NewFileConfigProvider(paths)
	assert.Equal(t, len(provider.paths), len(paths))

	for i, p := range provider.paths {
		assert.Equal(t, p, paths[i])
	}
}

func TestCollect(t *testing.T) {
	paths := []string{"tests", "foo/bar"}
	provider := NewFileConfigProvider(paths)
	configs, err := provider.Collect()

	assert.Nil(t, err)
	assert.Equal(t, len(configs), 1)

	config := configs[0]
	assert.Equal(t, config.Name, "testcheck")
}

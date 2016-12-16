package dogstream

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Setup the test module

func TestLoadConfig(t *testing.T) {

	cfg, err := loadConfig("tests/doesnt_exist.yaml")
	assert.NotNil(t, err)

	cfg, err = loadConfig("tests/config_test.yaml")
	assert.Nil(t, err)

	fmt.Printf("Config: %v\n", cfg)
	p := cfg["/var/log/auth.log"]
	assert.NotNil(t, p)

	fmt.Printf("returned %d loaders\n", len(p))
	assert.Nil(t, p[0].Parse("foo", "bar"))
}

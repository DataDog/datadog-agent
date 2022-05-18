package initcontainer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildCommandParamWithArgs(t *testing.T) {
	name, args := buildCommandParam("superCmd --verbose path -i .")
	assert.Equal(t, "superCmd", name)
	assert.Equal(t, []string{"--verbose", "path", "-i", "."}, args)
}

func TestBuildCommandParam(t *testing.T) {
	name, args := buildCommandParam("superCmd")
	assert.Equal(t, "superCmd", name)
	assert.Equal(t, []string{}, args)
}

package util

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetRootNSPID(t *testing.T) {
	t.Run("HOST_PROC not set", func(t *testing.T) {
		assert.Equal(t, os.Getpid(), GetRootNSPID())
	})

	t.Run("HOST_PROC set but not available", func(t *testing.T) {
		prev := os.Getenv("HOST_PROC")
		t.Cleanup(func() {
			os.Setenv("HOST_PROC", prev)
		})

		os.Setenv("HOST_PROC", "/foo/bar")
		assert.Equal(t, 0, GetRootNSPID())
	})
}

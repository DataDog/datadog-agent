// +build linux

package util

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetRootNSPID(t *testing.T) {
	t.Run("HOST_PROC not set", func(t *testing.T) {
		pid, err := GetRootNSPID()
		assert.Nil(t, err)
		assert.Equal(t, os.Getpid(), pid)
	})

	t.Run("HOST_PROC set but not available", func(t *testing.T) {
		prev := os.Getenv("HOST_PROC")
		t.Cleanup(func() {
			os.Setenv("HOST_PROC", prev)
		})

		os.Setenv("HOST_PROC", "/foo/bar")
		pid, err := GetRootNSPID()
		assert.NotNil(t, err)
		assert.Equal(t, 0, pid)
	})
}

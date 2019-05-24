package util

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDebugfsMounted(t *testing.T) {
	validFile := `
tmpfs /run/user/1000 tmpfs rw,nosuid,nodev,relatime,size=101608k,mode=700,uid=1000,gid=1000 0 0
none /sys/kernel/debug debugfs rw,relatime 0 0
	`
	reader := strings.NewReader(validFile)
	assert.True(t, IsDebugfsMounted(reader))

	invalidFile := `
tmpfs /run/user/1000 tmpfs rw,nosuid,nodev,relatime,size=101608k,mode=700,uid=1000,gid=1000 0 0
tracefs /sys/kernel/debug/tracing tracefs rw,relatime 0 0
`
	reader = strings.NewReader(invalidFile)
	assert.False(t, IsDebugfsMounted(reader))
}

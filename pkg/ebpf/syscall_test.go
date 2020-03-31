package ebpf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSyscall(t *testing.T) {
	assert.True(t, IsSysCall("kprobe/sys_bind"))
	assert.True(t, IsSysCall("kretprobe/sys_socket"))
	assert.False(t, IsSysCall("kprobe/tcp_send"))
}

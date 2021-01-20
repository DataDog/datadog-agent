// +build linux_bpf

package ebpf

import (
	"runtime"
	"strings"
	"testing"

	"github.com/DataDog/ebpf/manager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChooseSyscall(t *testing.T) {
	c := NewDefaultConfig()

	_, err := c.ChooseSyscallProbe("wrongformat", "", "")
	assert.Error(t, err)

	_, err = c.ChooseSyscallProbe("nontracepoint/what/wrong", "", "")
	assert.Error(t, err)

	_, err = c.ChooseSyscallProbe("tracepoint/syscalls/sys_enter_bind", "", "wrongformat")
	assert.Error(t, err)

	// kprobe syscalls must match
	_, err = c.ChooseSyscallProbe("tracepoint/syscalls/sys_enter_bind", "kprobe/sys_bind/x64", "kprobe/sys_socket")
	assert.Error(t, err)

	tp, err := c.ChooseSyscallProbe("tracepoint/syscalls/sys_enter_bind", "kprobe/sys_bind/x64", "kprobe/sys_bind")
	require.NoError(t, err)

	fnName, err := manager.GetSyscallFnName("sys_bind")
	require.NoError(t, err)

	// intentionally leaving amd64/arm64 explicit to ensure they are included in the prefix map
	switch runtime.GOARCH {
	case "amd64":
		if strings.HasPrefix(fnName, indirectSyscallPrefixes[runtime.GOARCH]) {
			assert.Equal(t, "kprobe/sys_bind/x64", tp)
		} else {
			assert.Equal(t, "kprobe/sys_bind", tp)
		}
	case "arm64":
		if strings.HasPrefix(fnName, indirectSyscallPrefixes[runtime.GOARCH]) {
			assert.Equal(t, "kprobe/sys_bind/x64", tp)
		} else {
			assert.Equal(t, "kprobe/sys_bind", tp)
		}
	default:
		assert.Equal(t, "kprobe/sys_bind", tp)
	}

	c.EnableTracepoints = true
	tp, err = c.ChooseSyscallProbe("tracepoint/syscalls/sys_enter_bind", "kprobe/sys_bind/x64", "kprobe/sys_bind")
	require.NoError(t, err)

	assert.Equal(t, "tracepoint/syscalls/sys_enter_bind", tp)
}

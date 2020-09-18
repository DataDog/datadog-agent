// +build linux_bpf

package ebpf

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/ebpf/manager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"runtime"
	"strings"
	"testing"
)

func TestChooseSyscall(t *testing.T) {
	c := NewDefaultConfig()

	_, err := c.chooseSyscallProbe("wrongformat", "", "")
	assert.Error(t, err)

	_, err = c.chooseSyscallProbe("nontracepoint/what/wrong", "", "")
	assert.Error(t, err)

	_, err = c.chooseSyscallProbe(bytecode.TraceSysBindEnter, "", "wrongformat")
	assert.Error(t, err)

	// kprobe syscalls must match
	_, err = c.chooseSyscallProbe(bytecode.TraceSysBindEnter, bytecode.SysBindX64, bytecode.SysSocket)
	assert.Error(t, err)

	tp, err := c.chooseSyscallProbe(bytecode.TraceSysBindEnter, bytecode.SysBindX64, bytecode.SysBind)
	require.NoError(t, err)

	if runtime.GOARCH == "386" {
		assert.Equal(t, bytecode.SysBind, tp)
	} else {
		fnName, err := manager.GetSyscallFnName("sys_bind")
		require.NoError(t, err)
		if strings.HasPrefix(fnName, x64SyscallPrefix) {
			assert.Equal(t, bytecode.SysBindX64, tp)
		} else {
			assert.Equal(t, bytecode.SysBind, tp)
		}
	}

	c.EnableTracepoints = true
	tp, err = c.chooseSyscallProbe(bytecode.TraceSysBindEnter, bytecode.SysBindX64, bytecode.SysBind)
	require.NoError(t, err)

	assert.Equal(t, bytecode.TraceSysBindEnter, tp)
}

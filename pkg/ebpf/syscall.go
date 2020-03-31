package ebpf

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// GetSyscallPrefix gets a prefix which will be prepended to every syscall kprobe.
// for example, if getSyscallPrefix returns sys_, then the kprobe for bind()
// will be called _sys_bind.
// Adapted from a similar BCC function:
// https://github.com/iovisor/bcc/blob/5e123df1dd33cdff5798560e4f0390c69cdba00f/src/python/bcc/__init__.py#L623-L627
func GetSyscallPrefix() (string, error) {

	syscallPrefixes := []string{
		"__sys_",
		"sys_",
		"__x64_sys_",
		"__x32_compat_sys_",
		"__ia32_compat_sys_",
		"__arm64_sys_",
		"__s390x_sys_",
		"__s390_sys_",
	}

	kallsyms := path.Join(util.GetProcRoot(), "kallsyms")
	file, err := os.Open(kallsyms)
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		for _, prefix := range syscallPrefixes {
			if strings.HasSuffix(line, " "+prefix+"socket") {
				return prefix, nil
			}
		}
	}

	return "", fmt.Errorf("could not get syscall prefix")
}

// FixSyscallName takes in a syscall prexix (see GetSyscallPrefix()) and a system call kprobe
// name (like kprobe/sys_bind or kretprobe/sys_bind) and returns a kprobe name
// as gobpf expects it with the corrected prefix.
//
// this function needs to exist because the kprobes for syscalls have different names
// depending on which kernel/architecture we're running on.
//
// see get_syscall_fnname in bcc https://github.com/iovisor/bcc/blob/5e123df1dd33cdff5798560e4f0390c69cdba00f/src/python/bcc/__init__.py#L632-L634
func FixSyscallName(prefix string, name string) string {
	// see get_syscall_fname in bcc

	parts := strings.Split(name, "/")
	probeType := parts[0]
	rawName := strings.TrimPrefix(parts[1], "sys_")

	out := probeType + "/" + prefix + rawName

	return out
}

// IsSysCall determines whether the Kprobe refers to a syscall
func IsSysCall(name string) bool {
	parts := strings.Split(name, "/")
	if len(parts) != 2 {
		return false
	}
	return strings.HasPrefix(parts[1], "sys_")
}

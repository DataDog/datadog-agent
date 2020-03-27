// +build linux_bpf

package ebpf

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func verifyOSVersion(kernelCode uint32, platform string, exclusionList []string) (bool, string) {
	for _, version := range exclusionList {
		if code := stringToKernelCode(version); code == kernelCode {
			return false, fmt.Sprintf(
				"current kernel version (%s) is in the exclusion list: %s (list: %+v)",
				kernelCodeToString(kernelCode),
				version,
				exclusionList,
			)
		}
	}

	// Hardcoded exclusion list
	if platform == "" {
		// If we can't retrieve the platform just return true to avoid blocking the tracer from running
		return true, ""
	}

	// using eBPF causes kernel panic for linux kernel version 4.4.114 ~ 4.4.127
	if isLinuxAWSUbuntu(platform) || isUbuntu(platform) {
		if kernelCode >= linuxKernelVersionCode(4, 4, 114) && kernelCode <= linuxKernelVersionCode(4, 4, 127) {
			return false, fmt.Sprintf("Known bug for kernel %s on platform %s, see: \n- https://bugs.launchpad.net/ubuntu/+source/linux/+bug/1763454", kernelCodeToString(kernelCode), platform)
		}
	}

	missing, err := verifyKernelFuncs(path.Join(util.GetProcRoot(), "kallsyms"))
	if err != nil {
		log.Warnf("error reading /proc/kallsyms file: %s (check your kernel version, current is: %s)", err, kernelCodeToString(kernelCode))
		// If we can't read the /proc/kallsyms file let's just return true to avoid blocking the tracer from running
		return true, ""
	}
	if len(missing) == 0 {
		return true, ""
	}
	errMsg := fmt.Sprintf("Kernel unsupported (%s) - ", kernelCodeToString(kernelCode))
	errMsg += fmt.Sprintf("required functions missing: %s", strings.Join(missing, ", "))
	return false, errMsg
}

// getSyscallPrefix gets a prefix which will be prepended to every syscall kprobe.
// for example, if getSyscallPrefix returns sys_, then the kprobe for bind()
// will be called _sys_bind.
// Adapted from a similar BCC function:
// https://github.com/iovisor/bcc/blob/5e123df1dd33cdff5798560e4f0390c69cdba00f/src/python/bcc/__init__.py#L623-L627
func getSyscallPrefix() (string, error) {

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

// takes in a syscall prexix (see getSyscallPrefix()) and a system call kprobe
// name (like kprobe/sys_bind or kretprobe/sys_bind) and returns a kprobe name
// as gobpf expects it with the corrected prefix.
//
// this function needs to exist because the kprobes for syscalls have different names
// depending on which kernel/architecture we're running on.
//
// see get_syscall_fnname in bcc https://github.com/iovisor/bcc/blob/5e123df1dd33cdff5798560e4f0390c69cdba00f/src/python/bcc/__init__.py#L632-L634
func fixSyscallName(prefix string, name KProbeName) string {
	// see get_syscall_fname in bcc

	parts := strings.Split(string(name), "/")
	probeType := parts[0]
	rawName := strings.TrimPrefix(parts[1], "sys_")

	out := probeType + "/" + prefix + rawName

	return out
}

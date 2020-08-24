// +build linux_bpf

package bytecode

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"
)

func getKernelIncludePaths() ([]string, error) {
	version, err := kernelVersion()
	if err != nil {
		return nil, fmt.Errorf("unable to determine kernel version: %s", err)
	}

	arch := kernelArch()
	if arch == "" {
		return nil, fmt.Errorf("unable to detect system architecture")
	}

	var includePaths []string
	for _, base := range baseDirs(version) {
		for _, sub := range subDirs(arch) {
			if dir := path.Join(base, sub); isDirectory(dir) {
				includePaths = append(includePaths, dir)
			}
		}
	}

	return includePaths, nil
}

func isDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileInfo.IsDir()
}

func baseDirs(kversion string) []string {
	return []string{
		fmt.Sprintf("/usr/src/kernels/%s", kversion),
		fmt.Sprintf("/usr/src/linux-headers-%s", kversion),
	}
}

func subDirs(arch string) []string {
	return []string{
		"include",
		"include/uapi",
		"include/generated/uapi",
		fmt.Sprintf("arch/%s/include", arch),
		fmt.Sprintf("arch/%s/include/uapi", arch),
		fmt.Sprintf("arch/%s/include/generated", arch),
	}
}

func kernelArch() string {
	switch runtime.GOARCH {
	case "386", "amd64":
		return "x86"
	case "arm", "armbe":
		return "arm"
	case "arm64", "arm64be":
		return "arm64"
	case "mips", "mipsle", "mips64", "mips64le":
		return "mips"
	case "ppc", "ppc64", "ppc64le":
		return "powerpc"
	case "riscv", "riscv64":
		return "riscv"
	case "s390", "s390x":
		return "s390"
	case "sparc", "sparc64":
		return "sparc64"
	default:
		return ""
	}
}

func kernelVersion() (string, error) {
	var uname unix.Utsname
	err := unix.Uname(&uname)
	if err != nil {
		return "", err
	}

	return strings.Trim(string(uname.Release[:]), "\x00"), nil
}

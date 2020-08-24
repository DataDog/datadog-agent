// +build linux_bpf

package bytecode

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
)

var baseDirGlobs = []string{
	"/usr/src/kernels/*",
	"/usr/src/linux-*",
}

func getKernelIncludePaths() []string {
	// First determine all base directories containing headers
	var matches []string
	for _, glob := range baseDirGlobs {
		matches, _ = filepath.Glob(glob)
		if len(matches) > 0 {
			break
		}
	}

	var baseDirs []string
	for _, m := range matches {
		if isDirectory(m) {
			baseDirs = append(baseDirs, m)
		}
	}

	// Now explicitly include the the set of subdirectories for each base entry
	arch := getKernelArch()
	subDirs := getHeaderSubDirs(arch)
	var includePaths []string
	for _, base := range baseDirs {
		for _, sub := range subDirs {
			includePaths = append(includePaths, path.Join(base, sub))
		}
	}

	return includePaths
}

func isDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileInfo.IsDir()
}

func getHeaderSubDirs(arch string) []string {
	return []string{
		"include",
		"include/uapi",
		"include/generated/uapi",
		fmt.Sprintf("arch/%s/include", arch),
		fmt.Sprintf("arch/%s/include/uapi", arch),
		fmt.Sprintf("arch/%s/include/generated", arch),
	}
}

func getKernelArch() string {
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

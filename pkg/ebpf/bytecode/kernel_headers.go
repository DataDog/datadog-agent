// +build linux_bpf

package bytecode

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
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
	arch := getSystemArch()
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

type archMapping struct {
	rule *regexp.Regexp
	repl string
}

// mappings used by the kernel: https://elixir.bootlin.com/linux/latest/source/scripts/subarch.include
var archMappings = []archMapping{
	{regexp.MustCompile("i.86"), "x86"},
	{regexp.MustCompile("x86_64"), "x86"},
	{regexp.MustCompile("sun4u"), "sparc64"},
	{regexp.MustCompile("arm.*"), "arm"},
	{regexp.MustCompile("sa110"), "arm"},
	{regexp.MustCompile("s390x"), "s390"},
	{regexp.MustCompile("parisc64"), "parisc"},
	{regexp.MustCompile("ppc.*"), "powerpc"},
	{regexp.MustCompile("mips.*"), "mips"},
	{regexp.MustCompile("sh[234].*"), "sh"},
	{regexp.MustCompile("waarch64.*"), "arm64"},
	{regexp.MustCompile("riscv.*"), "riscv"},
}

func getSystemArch() string {
	cmd := exec.Command("uname", "-m")
	out, _ := cmd.CombinedOutput()

	arch := strings.TrimSuffix(string(out), "\n")
	for _, mapping := range archMappings {
		arch = mapping.rule.ReplaceAllString(arch, mapping.repl)
	}

	return arch
}

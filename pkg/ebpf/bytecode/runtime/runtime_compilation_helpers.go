// +build linux_bpf

package runtime

import (
	"crypto/sha256"
	"fmt"
)

var (
	defaultFlags = []string{
		"-DCONFIG_64BIT",
		"-D__BPF_TRACING__",
		`-DKBUILD_MODNAME="ddsysprobe"`,
		"-Wno-unused-value",
		"-Wno-pointer-sign",
		"-Wno-compare-distinct-pointer-types",
		"-Wunused",
		"-Wall",
		"-Werror",
	}
)

func ComputeFlagsAndHash(additionalFlags []string) ([]string, string) {
	flags := make([]string, len(defaultFlags)+len(additionalFlags))
	copy(flags, defaultFlags)
	copy(flags[len(defaultFlags):], additionalFlags)

	flagHash := hashFlags(flags)
	return flags, flagHash
}

func hashFlags(flags []string) string {
	h := sha256.New()
	for _, f := range flags {
		h.Write([]byte(f))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

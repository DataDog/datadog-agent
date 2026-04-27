// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package dnfv2

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func computeBuiltinVariables(releaseVersion string) (map[string]string, error) {
	arch, baseArch, err := computeArch()
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"arch":       arch,
		"basearch":   baseArch,
		"releasever": releaseVersion,
	}, nil
}

var baseArchMapping = invertMap(map[string][]string{
	"aarch64":     {"aarch64"},
	"alpha":       {"alpha", "alphaev4", "alphaev45", "alphaev5", "alphaev56", "alphaev6", "alphaev67", "alphaev68", "alphaev7", "alphapca56"},
	"arm":         {"armv5tejl", "armv5tel", "armv5tl", "armv6l", "armv7l", "armv8l"},
	"armhfp":      {"armv6hl", "armv7hl", "armv7hnl", "armv8hl"},
	"i386":        {"i386", "athlon", "geode", "i386", "i486", "i586", "i686"},
	"ia64":        {"ia64"},
	"mips":        {"mips"},
	"mipsel":      {"mipsel"},
	"mips64":      {"mips64"},
	"mips64el":    {"mips64el"},
	"loongarch64": {"loongarch64"},
	"noarch":      {"noarch"},
	"ppc":         {"ppc"},
	"ppc64":       {"ppc64", "ppc64iseries", "ppc64p7", "ppc64pseries"},
	"ppc64le":     {"ppc64le"},
	"riscv32":     {"riscv32"},
	"riscv64":     {"riscv64"},
	"riscv128":    {"riscv128"},
	"s390":        {"s390"},
	"s390x":       {"s390x"},
	"sh3":         {"sh3"},
	"sh4":         {"sh4", "sh4a"},
	"sparc":       {"sparc", "sparc64", "sparc64v", "sparcv8", "sparcv9", "sparcv9v"},
	"x86_64":      {"x86_64", "amd64", "ia32e"},
})

func invertMap(direct map[string][]string) map[string]string {
	res := make(map[string]string, len(direct))
	for k, v := range direct {
		for _, subv := range v {
			res[subv] = k
		}
	}
	return res
}

func computeArch() (string, string, error) {
	arch, err := kernel.Machine()
	if err != nil {
		return "", "", err
	}

	baseArch, ok := baseArchMapping[arch]
	if !ok {
		return "", "", fmt.Errorf("no basearch for %s", arch)
	}

	return arch, baseArch, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux_bpf

package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/compiler"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CompiledOutput is the interface for a compiled output from runtime compilation
type CompiledOutput interface {
	io.Reader
	io.ReaderAt
	io.Closer
}

var defaultFlags = []string{
	"-DCOMPILE_RUNTIME",
	"-D__KERNEL__",
	"-DCONFIG_64BIT",
	"-D__BPF_TRACING__",
	`-DKBUILD_MODNAME="ddsysprobe"`,
	"-Wno-unused-value",
	"-Wno-pointer-sign",
	"-Wno-compare-distinct-pointer-types",
	"-Wunused",
	"-Wall",
	"-Werror",
	"-emit-llvm",
	"-O2",
	"-fno-stack-protector",
	"-fno-color-diagnostics",
	"-fno-unwind-tables",
	"-fno-asynchronous-unwind-tables",
	"-fno-jump-tables",
	"-nostdinc",
}

// compileToObjectFile compiles the input ebpf program & returns the compiled output
func compileToObjectFile(inFile, outputDir, filename, inHash string, additionalFlags, kernelHeaders []string) (CompiledOutput, CompilationResult, error) {
	flags, flagHash := computeFlagsAndHash(additionalFlags)

	outputFile, err := getOutputFilePath(outputDir, filename, inHash, flagHash)
	if err != nil {
		return nil, outputFileErr, fmt.Errorf("unable to get output file path: %w", err)
	}

	var result CompilationResult
	if _, err := os.Stat(outputFile); err != nil {
		if !os.IsNotExist(err) {
			return nil, outputFileErr, fmt.Errorf("error stat-ing output file %s: %w", outputFile, err)
		}

		kv, err := kernel.HostVersion()
		if err != nil {
			return nil, kernelVersionErr, fmt.Errorf("unable to get kernel version: %w", err)
		}
		family, err := kernel.Family()
		if err != nil {
			return nil, kernelVersionErr, fmt.Errorf("unable to get kernel family: %w", err)
		}

		// RHEL platforms back-ported the __BPF_FUNC_MAPPER macro, so we can always use the dynamic method there
		if len(kernelHeaders) > 0 && (kv >= kernel.VersionCode(4, 10, 0) || family == "rhel") {
			var helperPath string
			helperPath, err = includeHelperAvailability(kernelHeaders)
			if err != nil {
				return nil, compilationErr, fmt.Errorf("error getting helper availability: %w", err)
			}
			defer os.Remove(helperPath)
			flags = append(flags, fmt.Sprintf("-include%s", helperPath))
		}

		log.Debugf("compiling runtime version of %s to %s", filename, outputFile)
		if err := compiler.CompileToObjectFile(inFile, outputFile, flags, kernelHeaders); err != nil {
			return nil, compilationErr, fmt.Errorf("failed to compile runtime version of %s: %s", filename, err)
		}

		log.Infof("successfully compiled runtime version of %s", filename)
		result = compilationSuccess
	} else {
		log.Debugf("previously compiled runtime version of %s exists at %s", filename, outputFile)
		log.Infof("found previously compiled runtime version of %s", filename)
		result = compiledOutputFound
	}

	err = bytecode.VerifyAssetPermissions(outputFile)
	if err != nil {
		return nil, outputFileErr, err
	}

	out, err := os.Open(outputFile)
	if err != nil {
		return nil, resultReadErr, err
	}
	return out, result, nil
}

func computeFlagsAndHash(additionalFlags []string) ([]string, string) {
	flags := make([]string, 0, len(defaultFlags)+len(additionalFlags)+1)
	flags = append(flags, fmt.Sprintf("-D__TARGET_ARCH_%s", kernel.Arch()))
	flags = append(flags, defaultFlags...)
	flags = append(flags, additionalFlags...)

	hasher := sha256.New()
	for _, f := range flags {
		hasher.Write([]byte(f))
	}
	flagHash := fmt.Sprintf("%x", hasher.Sum(nil))

	return flags, flagHash
}

func getOutputFilePath(outputDir, filename, inputHash, flagHash string) (string, error) {
	// filename includes uname hash, input file hash, and cflags hash
	// this ensures we re-compile when either of the input changes
	baseName := strings.TrimSuffix(filename, filepath.Ext(filename))

	unameHash, err := getUnameHash()
	if err != nil {
		return "", err
	}

	outputFile := filepath.Join(outputDir, fmt.Sprintf("%s-%s-%s-%s.o", baseName, unameHash, inputHash, flagHash))
	return outputFile, nil
}

// getUnameHash returns a sha256 hash of the uname release and version
func getUnameHash() (string, error) {
	// we use the raw uname instead of the kernel version, because some kernel versions
	// can be clamped to 255 thus causing collisions
	r, err := kernel.Release()
	if err != nil {
		return "", err
	}
	v, err := kernel.UnameVersion()
	if err != nil {
		return "", err
	}
	return sha256hex([]byte(r + v))
}

// sha256hex returns the hex string of the sha256 of the provided buffer
func sha256hex(buf []byte) (string, error) {
	hasher := sha256.New()
	if _, err := hasher.Write(buf); err != nil {
		return "", fmt.Errorf("unable to get sha256 hash: %w", err)
	}
	cCodeHash := hasher.Sum(nil)
	return hex.EncodeToString(cCodeHash), nil
}

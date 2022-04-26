//go:build linux_bpf
// +build linux_bpf

package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/compiler"
	"github.com/DataDog/datadog-agent/pkg/security/log"
	"golang.org/x/sys/unix"
)

type CompiledOutput interface {
	io.Reader
	io.ReaderAt
	io.Closer
}

var defaultFlags = []string{
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

// GetNetworkAssetCFlags returns the cflags required for compilation based on the given network config
func GetNetworkAssetCFlags(collectIPv6Conns, bpfDebug bool) []string {
	var cflags []string
	if collectIPv6Conns {
		cflags = append(cflags, "-DFEATURE_IPV6_ENABLED")
	}
	if bpfDebug {
		cflags = append(cflags, "-DDEBUG=1")
	}
	return cflags
}

// GetSecurityAssetCFlags returns the cflags required for compilation of runtime security assets
func GetSecurityAssetCFlags(useSyscallWrapper bool) []string {
	var cflags []string
	if useSyscallWrapper {
		cflags = append(cflags, "-DUSE_SYSCALL_WRAPPER=1")
	} else {
		cflags = append(cflags, "-DUSE_SYSCALL_WRAPPER=0")
	}
	return cflags
}

func compileToObjectFile(in io.Reader, outputDir, filename, inHash string, additionalFlags, kernelHeaders []string, tm *RuntimeCompilationTelemetry) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		tm.compilationResult = outputDirErr
		return fmt.Errorf("unable to create compiler output directory %s: %w", outputDir, err)
	}

	flags, flagHash := computeFlagsAndHash(additionalFlags)

	outputFile, err := getOutputFilePath(outputDir, filename, inHash, flagHash)
	if err != nil {
		tm.compilationResult = outputFileErr
		return fmt.Errorf("unable to get output file path: %w", err)
	}

	if _, err := os.Stat(outputFile); err != nil {
		if !os.IsNotExist(err) {
			tm.compilationResult = outputFileErr
			return fmt.Errorf("error stat-ing output file %s: %w", outputFile, err)
		}

		if err := compiler.CompileToObjectFile(in, outputFile, flags, kernelHeaders); err != nil {
			tm.compilationResult = compilationErr
			return fmt.Errorf("failed to compile runtime version of %s: %s", filename, err)
		}

		tm.compilationResult = compilationSuccess
		log.Infof("successfully compiled runtime version of %s", filename)
	} else {
		tm.compilationResult = compiledOutputFound
		log.Infof("found previously compiled runtime version of %s", filename)
	}
	return nil
}

func computeFlagsAndHash(additionalFlags []string) ([]string, string) {
	flags := make([]string, len(defaultFlags)+len(additionalFlags))
	copy(flags, defaultFlags)
	copy(flags[len(defaultFlags):], additionalFlags)

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

func openCompiledOutput(filename, outputFile string) (CompiledOutput, error) {
	if _, err := os.Stat(outputFile); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%s has not been compiled", filename)
	}

	err := bytecode.VerifyAssetPermissions(outputFile)
	if err != nil {
		return nil, err
	}

	out, err := os.Open(outputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read compiled output: %w", err)
	}

	return out, nil
}

// getUnameHash returns a sha256 hash of the uname release and version
func getUnameHash() (string, error) {
	// we use the raw uname instead of the kernel version, because some kernel versions
	// can be clamped to 255 thus causing collisions
	var uname unix.Utsname
	if err := unix.Uname(&uname); err != nil {
		return "", fmt.Errorf("unable to get kernel version: %w", err)
	}

	var rv string
	rv += unix.ByteSliceToString(uname.Release[:])
	rv += unix.ByteSliceToString(uname.Version[:])
	return sha256hex([]byte(rv))
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

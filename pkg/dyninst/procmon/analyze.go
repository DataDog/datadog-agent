// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procmon

import (
	"bytes"
	"errors"
	"fmt"
	"iter"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// TODO: At some future point we may want to find a build ID and use that to
// cache the properties of the binary.
//
// [0] https://opentelemetry.io/docs/specs/otel/profiles/mappings/#algorithm-for-processexecutablebuild_idhtlhash

// analyzeProcess performs light analysis of the process and its binary
// to determine if it's interesting, and what its executable is.
func analyzeProcess(
	pid uint32, procfsRoot string,
) (processAnalysis, error) {
	ddEnv, err := analyzeEnviron(int32(pid), procfsRoot)
	if err != nil {
		return processAnalysis{}, fmt.Errorf(
			"failed to check if process %d is interesting: %w", pid, err,
		)
	}
	if ddEnv.serviceName == "" || !ddEnv.diEnabled {
		log.Tracef(
			"process %d is not interesting: service name is %q, %s=%t",
			pid, ddEnv.serviceName, ddDynInstEnabledEnvVar, ddEnv.diEnabled,
		)
		return processAnalysis{interesting: false}, nil
	}

	exeLinkPath := path.Join(procfsRoot, strconv.Itoa(int(pid)), "exe")
	exePath, err := os.Readlink(exeLinkPath)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, syscall.ESRCH) {
			return processAnalysis{}, nil
		}
		return processAnalysis{}, fmt.Errorf(
			"failed to open exe link for pid %d: %w", pid, err,
		)
	}

	exeFile, err := os.Open(exePath)
	if os.IsNotExist(err) || os.IsPermission(err) {
		// Try to open the exe under the proc root which can work when the
		// file exists inside a container.
		exePath = path.Join(
			procfsRoot, strconv.Itoa(int(pid)), "root",
			strings.TrimPrefix(exePath, "/"),
		)
		if log.ShouldLog(log.TraceLvl) {
			exePath := exePath
			log.Tracef(
				"exe for pid %d does not exist in root fs, trying under proc_root: %s",
				pid, exePath,
			)
		}
		var rootErr error
		exeFile, rootErr = os.Open(exePath)
		if rootErr != nil {
			err = errors.Join(err, rootErr)
		} else {
			// If we found the exe under the proc root, we can ignore the
			// original error.
			err = nil
		}
	}
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			err = fmt.Errorf("failed to open exe: %w", err)
		} else {
			err = nil
		}
		return processAnalysis{}, err
	}
	defer exeFile.Close()

	isGo, err := isGoElfBinaryWithDDTraceGo(exeFile)
	if errors.Is(err, os.ErrNotExist) {
		isGo, err = false, nil
	}
	if !isGo || err != nil {
		return processAnalysis{}, err
	}

	exe := Executable{Path: exePath}
	st, err := exeFile.Stat()
	if err != nil {
		return processAnalysis{}, fmt.Errorf(
			"failed to stat exe link for pid %v: %w", pid, err,
		)
	}
	if st, ok := st.Sys().(*syscall.Stat_t); ok {
		exe.Key = FileKey{
			FileHandle: FileHandle{
				Dev: uint64(st.Dev),
				Ino: st.Ino,
			},
			LastModified: st.Mtim,
		}
	}

	return processAnalysis{
		service:     ddEnv.serviceName,
		exe:         exe,
		interesting: true,
		gitInfo: GitInfo{
			CommitSha:     ddEnv.gitCommitSha,
			RepositoryURL: ddEnv.gitRepositoryURL,
		},
	}, nil
}

type ddEnvVars struct {
	serviceName      string
	diEnabled        bool
	gitCommitSha     string
	gitRepositoryURL string
}

const ddServiceEnvVar = "DD_SERVICE"
const ddDynInstEnabledEnvVar = "DD_DYNAMIC_INSTRUMENTATION_ENABLED"
const ddGitCommitShaEnvVar = "DD_GIT_COMMIT_SHA"
const ddGitRepositoryURLEnvVar = "DD_GIT_REPOSITORY_URL"

func analyzeEnviron(pid int32, procfsRoot string) (ddEnvVars, error) {
	procEnv := path.Join(procfsRoot, strconv.Itoa(int(pid)), "environ")
	env, err := os.ReadFile(procEnv)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, syscall.ESRCH) {
			return ddEnvVars{}, nil
		}
		return ddEnvVars{}, fmt.Errorf(
			"failed to read proc env at %s: %w", procEnv, err,
		)
	}
	var ddEnv ddEnvVars
	for envVar, val := range envVars(env) {
		switch envVar {
		case ddServiceEnvVar:
			ddEnv.serviceName = val
		case ddDynInstEnabledEnvVar:
			ddEnv.diEnabled, _ = strconv.ParseBool(val)
		case ddGitCommitShaEnvVar:
			ddEnv.gitCommitSha = val
		case ddGitRepositoryURLEnvVar:
			ddEnv.gitRepositoryURL = val
		}
	}
	return ddEnv, nil
}

// envVars returns an iterator over the environment variables in the given
// procfs environment file.
func envVars(procEnviron []byte) iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		cur := procEnviron
		for len(cur) > 0 {
			curVar := cur
			if idx := bytes.IndexByte(cur, 0); idx != -1 {
				curVar = cur[:idx]
				cur = cur[idx+1:]
			} else {
				cur = nil
			}
			eqIdx := bytes.IndexByte(curVar, '=')
			if eqIdx == -1 { // shouldn't happen, just skip it
				continue
			}
			if !yield(string(curVar[:eqIdx]), string(curVar[eqIdx+1:])) {
				return
			}
		}
	}
}

var goSections = map[string]struct{}{
	".gosymtab":     {},
	".gopclntab":    {},
	".go.buildinfo": {},
}

// isGoElfBinaryWithDDTraceGo returns true if the given executable is an ELF
// file that contains go sections and debug info, and contains the relevant
// dd-trace-go symbols we need to instrument.
//
// In the future we may want to look and see here if there's a relevant
// version of the trace agent that we care about, but for now we leave
// that to later analysis of dwarf.
func isGoElfBinaryWithDDTraceGo(f *os.File) (bool, error) {
	elfFile, err := object.NewMMappingElfFile(f.Name())
	if err != nil {
		log.Tracef("isGoElfBinary(%s): not an ELF file: %v", f.Name(), err)
		return false, nil
	}
	defer elfFile.Close() // no-op, but why not

	var symtabSection *safeelf.Section
	var hasDebugInfo, hasGoSections bool
	for _, section := range elfFile.Elf.Sections {
		if _, ok := goSections[section.Name]; ok {
			hasGoSections = true
		}
		if section.Name == ".debug_info" {
			hasDebugInfo = true
		}
		if section.Name == ".symtab" {
			symtabSection = section
		}
	}
	if log.ShouldLog(log.TraceLvl) {
		log.Tracef(
			"isGoElfBinary(%s): hasGoSections: %v, hasDebugInfo: %v, symtabSection: %v",
			f.Name(), hasGoSections, hasDebugInfo, symtabSection == nil,
		)
	}
	if !hasGoSections || !hasDebugInfo || symtabSection == nil {
		return false, nil
	}

	// This is a pretty rough heuristic but it's cheap. The way it works is to
	// find the string table for the symbol table and then scan it for the
	// strings corresponding to the symbols we might care about.
	symtabStringsSectionIdx := symtabSection.Link
	if symtabStringsSectionIdx >= uint32(len(elfFile.Elf.Sections)) {
		return false, nil
	}
	symtabStringsSection := elfFile.Elf.Sections[symtabStringsSectionIdx]
	symtabStrings, err := elfFile.MMap(symtabStringsSection, 0, symtabStringsSection.Size)
	if err != nil {
		return false, fmt.Errorf("failed to get symbols: %w", err)
	}
	defer symtabStrings.Close()
	return bytes.Contains(symtabStrings.Data, ddTraceSymbolSuffix), nil
}

var ddTraceSymbolSuffix = []byte("ddtrace/tracer.passProbeConfiguration")

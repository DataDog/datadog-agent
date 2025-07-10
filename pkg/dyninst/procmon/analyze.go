// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procmon

import (
	"bytes"
	"fmt"
	"iter"
	"os"
	"path"
	"strconv"
	"syscall"

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
		log.Tracef("process %d is not interesting: service name is %q, DD_DYNINST_ENABLED is %v", pid, ddEnv.serviceName, ddEnv.diEnabled)
		return processAnalysis{interesting: false}, nil
	}

	exePath := path.Join(procfsRoot, strconv.Itoa(int(pid)), "exe")
	exeLink, err := os.Readlink(exePath)
	if err != nil {
		return processAnalysis{}, fmt.Errorf(
			"failed to open exe link for pid %d: %w", pid, err,
		)
	}

	isGo, err := isGoElfBinary(exeLink)
	if err != nil {
		return processAnalysis{}, fmt.Errorf(
			"failed to check if exe is go binary: %w", err,
		)
	}
	if !isGo {
		return processAnalysis{interesting: false}, nil
	}

	exe := Executable{Path: exePath}
	st, err := os.Stat(exeLink)
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
	}, nil
}

type ddEnvVars struct {
	serviceName string
	diEnabled   bool
}

const ddServiceEnvVar = "DD_SERVICE"
const ddDynInstEnabledEnvVar = "DD_DYNAMIC_INSTRUMENTATION_ENABLED"

func analyzeEnviron(pid int32, procfsRoot string) (ddEnvVars, error) {
	procEnv := path.Join(procfsRoot, strconv.Itoa(int(pid)), "environ")
	env, err := os.ReadFile(procEnv)
	if err != nil {
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
		}
		if ddEnv.serviceName != "" && ddEnv.diEnabled {
			break
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

// isGoElfBinary returns true if the given executable is an ELF file that
// contains go sections and debug info.
//
// In the future we may want to look and see here if there's a relevant
// version of the trace agent that we care about, but for now we leave
// that to later analysis of dwarf.
func isGoElfBinary(exePath string) (bool, error) {
	f, err := os.Open(exePath)
	if err != nil {
		return false, fmt.Errorf("failed to open exe: %w", err)
	}
	defer f.Close()

	elfFile, err := safeelf.NewFile(f)
	if err != nil {
		log.Tracef("isGoElfBinary(%s): not an ELF file: %v", exePath, err)
		return false, nil
	}
	defer elfFile.Close() // no-op, but why not

	var hasDebugInfo, hasGoSections bool
	for _, section := range elfFile.Sections {
		if _, ok := goSections[section.Name]; ok {
			hasGoSections = true
		}
		if section.Name == ".debug_info" {
			hasDebugInfo = true
		}
	}
	log.Tracef(
		"isGoElfBinary(%s): hasGoSections: %v, hasDebugInfo: %v",
		exePath, hasGoSections, hasDebugInfo,
	)
	return hasGoSections && hasDebugInfo, nil
}

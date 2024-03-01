// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parser

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode"

	"github.com/Masterminds/semver"
	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/process/metadata"
	nodejsparser "github.com/DataDog/datadog-agent/pkg/process/metadata/parser/nodejs"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type serviceExtractorFn func(serviceExtractor *ServiceExtractor, process *procutil.Process, args []string) string

const (
	javaJarFlag         = "-jar"
	javaJarExtension    = ".jar"
	javaModuleFlag      = "--module"
	javaModuleFlagShort = "-m"
	javaSnapshotSuffix  = "-SNAPSHOT"
	javaApachePrefix    = "org.apache."
	dllSuffix           = ".dll"
)

var (
	javaAllowedFlags = []string{javaJarFlag, javaModuleFlag, javaModuleFlagShort}
)

// List of binaries that usually have additional process context of whats running
var binsWithContext = map[string]serviceExtractorFn{
	"python":     parseCommandContextPython,
	"python2.7":  parseCommandContextPython,
	"python3":    parseCommandContextPython,
	"python3.7":  parseCommandContextPython,
	"ruby2.3":    parseCommandContext,
	"ruby":       parseCommandContext,
	"java":       parseCommandContextJava,
	"java.exe":   parseCommandContextJava,
	"sudo":       parseCommandContext,
	"node":       parseCommandContextNodeJs,
	"node.exe":   parseCommandContextNodeJs,
	"dotnet":     parseCommandContextDotnet,
	"dotnet.exe": parseCommandContextDotnet,
}

var _ metadata.Extractor = &ServiceExtractor{}

// ServiceExtractor infers a service tag by extracting it from a process
type ServiceExtractor struct {
	enabled               bool
	useImprovedAlgorithm  bool
	useWindowsServiceName bool
	serviceByPID          map[int32]*serviceMetadata
	scmReader             *scmReader
}

type serviceMetadata struct {
	cmdline        []string
	serviceContext string
}

// WindowsServiceInfo represents service data that is parsed from the SCM. On non-Windows platforms these fields should always be empty.
// On Windows, multiple services can be binpacked into a single `svchost.exe`, which is why `ServiceName` and `DisplayName` are slices.
type WindowsServiceInfo struct {
	ServiceName []string
	DisplayName []string
}

// NewServiceExtractor instantiates a new service discovery extractor
func NewServiceExtractor(enabled, useWindowsServiceName, useImprovedAlgorithm bool) *ServiceExtractor {
	return &ServiceExtractor{
		enabled:               enabled,
		useImprovedAlgorithm:  useImprovedAlgorithm,
		useWindowsServiceName: useWindowsServiceName,
		serviceByPID:          make(map[int32]*serviceMetadata),
		scmReader:             newSCMReader(),
	}
}

//nolint:revive // TODO(PROC) Fix revive linter
func (d *ServiceExtractor) Extract(processes map[int32]*procutil.Process) {
	if !d.enabled {
		return
	}

	serviceByPID := make(map[int32]*serviceMetadata)

	for _, proc := range processes {
		if meta, seen := d.serviceByPID[proc.Pid]; seen {
			// check the service metadata is for the same process
			if len(proc.Cmdline) == len(meta.cmdline) {
				if len(proc.Cmdline) == 0 || proc.Cmdline[0] == meta.cmdline[0] {
					serviceByPID[proc.Pid] = meta
					continue
				}
			}
		}
		meta := d.extractServiceMetadata(proc)
		if meta != nil && log.ShouldLog(seelog.TraceLvl) {
			log.Tracef("detected service metadata: %v", meta)
		}
		serviceByPID[proc.Pid] = meta
	}

	d.serviceByPID = serviceByPID
}

//nolint:revive // TODO(PROC) Fix revive linter
func (d *ServiceExtractor) GetServiceContext(pid int32) []string {
	if !d.enabled {
		return nil
	}

	if runtime.GOOS == "windows" && d.useWindowsServiceName {
		tags, err := d.getWindowsServiceTags(pid)
		if err != nil {
			log.Warnf("Failed to get service data from SCM for pid %v:%v", pid, err.Error())
		}

		// Service tag was found from the SCM, return it.
		if len(tags) > 0 {
			if log.ShouldLog(seelog.TraceLvl) {
				log.Tracef("Found process_context from SCM for pid:%v service tags:%v", pid, tags)
			}
			return tags
		}
	}

	if meta, ok := d.serviceByPID[pid]; ok {
		return []string{meta.serviceContext}
	}
	return nil
}

func (d *ServiceExtractor) extractServiceMetadata(process *procutil.Process) *serviceMetadata {
	cmd := process.Cmdline
	if len(cmd) == 0 || len(cmd[0]) == 0 {
		return &serviceMetadata{
			cmdline: cmd,
		}
	}

	exe := cmd[0]
	// check if all args are packed into the first argument
	if len(cmd) == 1 {
		if idx := strings.IndexRune(exe, ' '); idx != -1 {
			exe = exe[0:idx]
			cmd = strings.Split(cmd[0], " ")
		}
	}

	// trim any quotes from the executable
	exe = strings.Trim(exe, "\"")

	// Extract executable from commandline args
	exe = trimColonRight(removeFilePath(exe))
	if !isRuneLetterAt(exe, 0) {
		exe = parseExeStartWithSymbol(exe)
	}

	if contextFn, ok := binsWithContext[exe]; ok {
		tag := contextFn(d, process, cmd[1:])
		return &serviceMetadata{
			cmdline:        cmd,
			serviceContext: "process_context:" + tag,
		}
	}

	// trim trailing file extensions
	if i := strings.LastIndex(exe, "."); i > 0 {
		exe = exe[:i]
	}

	return &serviceMetadata{
		cmdline:        cmd,
		serviceContext: "process_context:" + exe,
	}
}

// GetWindowsServiceTags returns the process_context associated with a process by scraping the SCM.
// If the service name is not found in the scm, a nil slice is returned.
func (d *ServiceExtractor) getWindowsServiceTags(pid int32) ([]string, error) {
	entry, err := d.scmReader.getServiceInfo(uint64(pid))
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	serviceTags := make([]string, 0, len(entry.ServiceName))
	for _, serviceName := range entry.ServiceName {
		serviceTags = append(serviceTags, "process_context:"+serviceName)
	}
	return serviceTags, nil
}

func removeFilePath(s string) string {
	if s != "" {
		return filepath.Base(s)
	}
	return s
}

// trimColonRight will remove any colon and it's associated value right of the string
func trimColonRight(s string) string {
	if i := strings.Index(s, ":"); i > 0 {
		return s[:i]
	}

	return s
}

func isRuneLetterAt(s string, position int) bool {
	return len(s) > position && unicode.IsLetter(rune(s[position]))
}

// parseExeStartWithSymbol deals with exe that starts with special chars like "(", "-" or "["
func parseExeStartWithSymbol(exe string) string {
	if exe == "" {
		return exe
	}
	// drop the first character
	result := exe[1:]
	// if last character is also special character, also drop it
	if result != "" && !isRuneLetterAt(result, len(result)-1) {
		result = result[:len(result)-1]
	}
	return result
}

// In most cases, the best context is the first non-argument / environment variable, if it exists
func parseCommandContext(_ *ServiceExtractor, _ *procutil.Process, args []string) string {
	var prevArgIsFlag bool

	for _, a := range args {
		hasFlagPrefix, isEnvVariable := strings.HasPrefix(a, "-"), strings.ContainsRune(a, '=')
		shouldSkipArg := prevArgIsFlag || hasFlagPrefix || isEnvVariable

		if !shouldSkipArg {
			if c := trimColonRight(removeFilePath(a)); isRuneLetterAt(c, 0) {
				return c
			}
		}

		prevArgIsFlag = hasFlagPrefix
	}

	return ""
}

func parseCommandContextPython(_ *ServiceExtractor, _ *procutil.Process, args []string) string {
	var (
		prevArgIsFlag bool
		moduleFlag    bool
	)

	for _, a := range args {
		hasFlagPrefix, isEnvVariable := strings.HasPrefix(a, "-"), strings.ContainsRune(a, '=')

		shouldSkipArg := prevArgIsFlag || hasFlagPrefix || isEnvVariable

		if !shouldSkipArg || moduleFlag {
			if c := trimColonRight(removeFilePath(a)); isRuneLetterAt(c, 0) {
				return c
			}
		}

		if hasFlagPrefix && a == "-m" {
			moduleFlag = true
		}

		prevArgIsFlag = hasFlagPrefix
	}

	return ""
}

func parseCommandContextJava(_ *ServiceExtractor, _ *procutil.Process, args []string) string {
	prevArgIsFlag := false

	// Look for dd.service
	if index := slices.IndexFunc(args, func(arg string) bool { return strings.HasPrefix(arg, "-Ddd.service=") }); index != -1 {
		return strings.TrimPrefix(args[index], "-Ddd.service=")
	}

	for _, a := range args {
		hasFlagPrefix := strings.HasPrefix(a, "-")
		includesAssignment := strings.ContainsRune(a, '=') ||
			strings.HasPrefix(a, "-X") ||
			strings.HasPrefix(a, "-javaagent:") ||
			strings.HasPrefix(a, "-verbose:")
		shouldSkipArg := prevArgIsFlag || hasFlagPrefix || includesAssignment
		if !shouldSkipArg {
			arg := removeFilePath(a)

			if arg = trimColonRight(arg); isRuneLetterAt(arg, 0) {
				if strings.HasSuffix(arg, javaJarExtension) {
					jarName := arg[:len(arg)-len(javaJarExtension)]
					if !strings.HasSuffix(jarName, javaSnapshotSuffix) {
						return jarName
					}
					jarName = jarName[:len(jarName)-len(javaSnapshotSuffix)]

					if idx := strings.LastIndex(jarName, "-"); idx != -1 {
						if _, err := semver.NewVersion(jarName[idx+1:]); err == nil {
							return jarName[:idx]
						}
					}
					return jarName
				}

				if strings.HasPrefix(arg, javaApachePrefix) {
					// take the project name after the package 'org.apache.' while stripping off the remaining package
					// and class name
					arg = arg[len(javaApachePrefix):]
					if idx := strings.Index(arg, "."); idx != -1 {
						return arg[:idx]
					}
				}
				if idx := strings.LastIndex(arg, "."); idx != -1 && idx+1 < len(arg) {
					// take just the class name without the package
					return arg[idx+1:]
				}

				return arg
			}
		}

		prevArgIsFlag = hasFlagPrefix && !includesAssignment && !slices.Contains(javaAllowedFlags, a)
	}

	return "java"
}

func parseCommandContextNodeJs(se *ServiceExtractor, process *procutil.Process, args []string) string {
	if !se.useImprovedAlgorithm {
		return "node"
	}
	skipNext := false
	for _, a := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(a, "-") {
			if a == "-r" || a == "--require" {
				// next arg can be a js file but not the entry point. skip it
				skipNext = !strings.ContainsRune(a, '=') // in this case the value is already in this arg
				continue
			}
		} else if strings.HasSuffix(strings.ToLower(a), ".js") {
			absFile := abs(filepath.Clean(a), process.Cwd)
			if _, err := os.Stat(absFile); err == nil {
				value, ok := nodejsparser.FindNameFromNearestPackageJSON(absFile)
				if ok {
					return value
				}
				break
			}
		}
	}
	return "node"
}

// abs returns the path itself if already absolute or the absolute path by joining cwd with path
// This is a variant of filepath.Abs since on windows it likely returns false when the drive/volume is missing
// hence, since we accept also paths, we test if the first char is a path separator
func abs(path string, cwd string) string {
	if !(filepath.IsAbs(path) || path[0] == os.PathSeparator) && len(cwd) > 0 {
		return filepath.Join(cwd, path)
	}
	return path
}

// parseCommandContextDotnet extracts metadata from a dotnet launcher command line
func parseCommandContextDotnet(se *ServiceExtractor, _ *procutil.Process, args []string) string {
	if !se.useImprovedAlgorithm {
		return "dotnet"
	}
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		// when running assembly's dll, the cli must be executed without command
		// https://learn.microsoft.com/en-us/dotnet/core/tools/dotnet-run#description
		if strings.HasSuffix(strings.ToLower(a), dllSuffix) {
			_, file := filepath.Split(a)
			return file[:len(file)-len(dllSuffix)]
		}
		// dotnet cli syntax is something like `dotnet <cmd> <args> <dll> <prog args>`
		// if the first non arg (`-v, --something, ...) is not a dll file, exit early since nothing is matching a dll execute case
		break
	}
	return "dotnet"
}

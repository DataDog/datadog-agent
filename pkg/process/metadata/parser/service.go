// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parser

import (
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/metadata"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type serviceExtractorFn func(args []string) string

const (
	javaJarFlag      = "-jar"
	javaJarExtension = ".jar"
	javaApachePrefix = "org.apache."
)

// List of binaries that usually have additional process context of whats running
var binsWithContext = map[string]serviceExtractorFn{
	"python":    parseCommandContextPython,
	"python2.7": parseCommandContextPython,
	"python3":   parseCommandContextPython,
	"python3.7": parseCommandContextPython,
	"ruby2.3":   parseCommandContext,
	"ruby":      parseCommandContext,
	"java":      parseCommandContextJava,
	"java.exe":  parseCommandContextJava,
	"sudo":      parseCommandContext,
}

var _ metadata.Extractor = &ServiceExtractor{}

// ServiceExtractor infers a service tag by extracting it from a process
type ServiceExtractor struct {
	enabled               bool
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
func NewServiceExtractor(sysProbeConfig ddconfig.ConfigReader) *ServiceExtractor {
	var (
		enabled               = sysProbeConfig.GetBool("service_monitoring_config.process_service_inference.enabled")
		useWindowsServiceName = sysProbeConfig.GetBool("service_monitoring_config.process_service_inference.use_windows_service_name")
	)
	return &ServiceExtractor{
		enabled:               enabled,
		useWindowsServiceName: useWindowsServiceName,
		serviceByPID:          make(map[int32]*serviceMetadata),
		scmReader:             newSCMReader(),
	}
}

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
		meta := extractServiceMetadata(proc.Cmdline)
		if meta != nil {
			log.Tracef("detected service metadata: %v", meta)
		}
		serviceByPID[proc.Pid] = meta
	}

	d.serviceByPID = serviceByPID
}

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
			log.Tracef("Found process_context from SCM for pid:%v service tags:%v", pid, tags)
			return tags
		}
	}

	if meta, ok := d.serviceByPID[pid]; ok {
		return []string{meta.serviceContext}
	}
	return nil
}

func extractServiceMetadata(cmd []string) *serviceMetadata {
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
		tag := contextFn(cmd[1:])
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
func parseCommandContext(args []string) string {
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

func parseCommandContextPython(args []string) string {
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

func parseCommandContextJava(args []string) string {
	prevArgIsFlag := false

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
					return arg[:len(arg)-len(javaJarExtension)]
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

		prevArgIsFlag = hasFlagPrefix && !includesAssignment && a != javaJarFlag
	}

	return ""
}

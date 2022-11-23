// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parser

import (
	"path/filepath"
	"strings"
	"unicode"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/metadata"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type serviceExtractorFn func(args []string) string

// List of binaries that usually have additional process context of whats running
var binsWithContext = map[string]serviceExtractorFn{
	"python":    parseCommandContextPython,
	"python2.7": parseCommandContextPython,
	"python3":   parseCommandContextPython,
	"python3.7": parseCommandContextPython,
	"ruby2.3":   parseCommandContext,
	"ruby":      parseCommandContext,
	"java":      parseCommandContext,
	"sudo":      parseCommandContext,
}

var _ metadata.Extractor = &ServiceExtractor{}

// ServiceExtractor infers a service tag by extracting it from a process
type ServiceExtractor struct {
	Enabled      bool
	serviceByPID map[int32]*serviceMetadata
}

type serviceMetadata struct {
	cmdline    []string
	serviceTag string
}

// NewServiceExtractor instantiates a new service discovery extractor
func NewServiceExtractor() *ServiceExtractor {
	enabled := ddconfig.Datadog.GetBool("service_monitoring_config.process_service_inference.enabled")
	return &ServiceExtractor{
		Enabled:      enabled,
		serviceByPID: make(map[int32]*serviceMetadata),
	}
}

func (d *ServiceExtractor) Type() string {
	return "serviceExtractor"
}

func (d *ServiceExtractor) Extract(p *procutil.Process) {
	if !d.Enabled {
		return
	}

	if meta, seen := d.serviceByPID[p.Pid]; seen {
		// check the service metadata is for the same process
		if len(p.Cmdline) == len(meta.cmdline) {
			if len(p.Cmdline) == 0 || p.Cmdline[0] == meta.cmdline[0] {
				return
			}
		}
	}
	meta := extractServiceMetadata(p.Cmdline)
	if meta != nil {
		log.Tracef("detected service metadata: %v", meta)
	}

	d.serviceByPID[p.Pid] = meta
}
func (d *ServiceExtractor) GetServiceTag(pid int32) string {
	if !d.Enabled {
		return ""
	}
	if meta, ok := d.serviceByPID[pid]; ok {
		return meta.serviceTag
	}
	return ""
}

func extractServiceMetadata(cmd []string) *serviceMetadata {
	if len(cmd) == 0 || len(cmd[0]) == 0 {
		return &serviceMetadata{
			cmdline: cmd,
		}
	}

	exe := cmd[0]
	// check if all args are packed into the first argument
	if idx := strings.IndexRune(exe, ' '); idx != -1 {
		exe = exe[0:idx]
		cmd = strings.Split(cmd[0], " ")
	}

	// Extract executable from commandline args
	exe = trimColonRight(removeFilePath(exe))
	if !isRuneLetterAt(exe, 0) {
		exe = parseExeStartWithSymbol(exe)
	}

	if contextFn, ok := binsWithContext[exe]; ok {
		tag := contextFn(cmd[1:])
		return &serviceMetadata{
			cmdline:    cmd,
			serviceTag: "service:" + tag,
		}
	}

	// trim trailing file extensions
	if i := strings.LastIndex(exe, "."); i > 0 {
		exe = exe[:i]
	}

	return &serviceMetadata{
		cmdline:    cmd,
		serviceTag: "service:" + exe,
	}
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

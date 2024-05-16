// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package usm provides functionality to detect the most appropriate service name for a process.
package usm

import (
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"unicode"

	"go.uber.org/zap"
)

type detectorCreatorFn func(ctx DetectionContext) detector

const (
	javaJarFlag      = "-jar"
	javaJarExtension = ".jar"
	javaApachePrefix = "org.apache."
	maxParseFileSize = 1024 * 1024
)

// ServiceMetadata holds information about a service.
type ServiceMetadata struct {
	Name            string
	AdditionalNames []string
	// for future usage: we can detect also the type, vendor, frameworks, etc
}

// NewServiceMetadata initializes ServiceMetadata.
func NewServiceMetadata(name string, additional ...string) ServiceMetadata {
	if len(additional) > 1 {
		// names are discovered in unpredictable order. We need to keep them sorted if we're going to join them
		slices.Sort(additional)
	}
	return ServiceMetadata{Name: name, AdditionalNames: additional}
}

// GetServiceKey returns the key for the service.
func (s ServiceMetadata) GetServiceKey() string {
	if len(s.AdditionalNames) > 0 {
		return strings.Join(s.AdditionalNames, "_")
	}
	return s.Name
}

type detector interface {
	detect(remainingArgs []string) (ServiceMetadata, bool)
}

type pythonDetector struct {
	ctx DetectionContext
}
type simpleDetector struct {
	ctx DetectionContext
}

func newPythonDetector(ctx DetectionContext) detector {
	return &pythonDetector{ctx: ctx}
}
func newSimpleDetector(ctx DetectionContext) detector {
	return &simpleDetector{ctx: ctx}
}

// DetectionContext allows to detect ServiceMetadata.
type DetectionContext struct {
	logger *zap.Logger
	args   []string
	envs   []string
	fs     fs.SubFS
}

// NewDetectionContext initializes DetectionContext.
func NewDetectionContext(logger *zap.Logger, args []string, envs []string, fs fs.SubFS) DetectionContext {
	return DetectionContext{
		logger: logger,
		args:   args,
		envs:   envs,
		fs:     fs,
	}
}

// workingDirFromEnvs returns the current working dir extracted from the PWD env
func workingDirFromEnvs(envs []string) (string, bool) {
	wd := ""
	for _, v := range envs {
		if strings.HasPrefix(v, "PWD=") {
			_, wd, _ = strings.Cut(v, "=")
			break
		}
	}
	return wd, len(wd) > 0
}

// abs returns the path itself if already absolute or the absolute path by joining cwd with path
func abs(p string, cwd string) string {
	if path.IsAbs(p) || len(cwd) == 0 {
		return p
	}
	return path.Join(cwd, p)
}

// canSafelyParse determines if a file's size is less than the maximum allowed to prevent OOM when parsing.
func canSafelyParse(file fs.File) (bool, error) {
	fi, err := file.Stat()
	if err != nil {
		return false, err
	}
	return fi.Size() <= maxParseFileSize, nil
}

// List of binaries that usually have additional process context of what's running
var binsWithContext = map[string]detectorCreatorFn{
	"python":    newPythonDetector,
	"python2.7": newPythonDetector,
	"python3":   newPythonDetector,
	"python3.7": newPythonDetector,
	"ruby2.3":   newSimpleDetector,
	"ruby":      newSimpleDetector,
	"java":      newJavaDetector,
	"sudo":      newSimpleDetector,
	"node":      newNodeDetector,
	"php":       newPhpDetector,
}

// ExtractServiceMetadata attempts to detect ServiceMetadata from the given process.
func ExtractServiceMetadata(ctx DetectionContext) (ServiceMetadata, bool) {
	cmd := ctx.args
	if len(cmd) == 0 || len(cmd[0]) == 0 {
		return ServiceMetadata{}, false
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

	if detectorProvider, ok := binsWithContext[exe]; ok {
		return detectorProvider(ctx).detect(cmd[1:])
	}

	// trim trailing file extensions
	if i := strings.LastIndex(exe, "."); i > 0 {
		exe = exe[:i]
	}

	return NewServiceMetadata(exe), true
}

func removeFilePath(s string) string {
	if s != "" {
		return path.Base(s)
	}
	return s
}

// trimColonRight will remove any colon and its associated value right of the string
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

func (simpleDetector) detect(args []string) (ServiceMetadata, bool) {
	// In most cases, the best context is the first non-argument / environment variable, if it exists
	var prevArgIsFlag bool

	for _, a := range args {
		hasFlagPrefix, isEnvVariable := strings.HasPrefix(a, "-"), strings.ContainsRune(a, '=')
		shouldSkipArg := prevArgIsFlag || hasFlagPrefix || isEnvVariable

		if !shouldSkipArg {
			if c := trimColonRight(removeFilePath(a)); isRuneLetterAt(c, 0) {
				return NewServiceMetadata(c), true
			}
		}

		prevArgIsFlag = hasFlagPrefix
	}

	return ServiceMetadata{}, false
}

func (pythonDetector) detect(args []string) (ServiceMetadata, bool) {
	var (
		prevArgIsFlag bool
		moduleFlag    bool
	)

	for _, a := range args {
		hasFlagPrefix, isEnvVariable := strings.HasPrefix(a, "-"), strings.ContainsRune(a, '=')

		shouldSkipArg := prevArgIsFlag || hasFlagPrefix || isEnvVariable

		if !shouldSkipArg || moduleFlag {
			if c := trimColonRight(removeFilePath(a)); isRuneLetterAt(c, 0) {
				return NewServiceMetadata(c), true
			}
		}

		if hasFlagPrefix && a == "-m" {
			moduleFlag = true
		}

		prevArgIsFlag = hasFlagPrefix
	}

	return ServiceMetadata{}, false
}

// RealFs implements real fs operations.
type RealFs struct{}

// Open calls os.Open.
func (RealFs) Open(name string) (fs.File, error) {
	return os.Open(name)
}

// Sub calls os.DirFS.
func (RealFs) Sub(dir string) (fs.FS, error) {
	return os.DirFS(dir), nil
}

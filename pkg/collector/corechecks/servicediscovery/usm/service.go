// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package usm provides functionality to detect the most appropriate service name for a process.
package usm

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/envs"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
)

type detectorCreatorFn func(ctx DetectionContext) detector

// DetectorContextMap is a map for passing data between the different detectors
// of the service discovery (i.e between the service name detector and the
// instrumentation detector)
type DetectorContextMap map[int]interface{}

// DetectorContextMap keys enum
const (
	// NodePackageJSONPath The path to the Node service's package.json
	NodePackageJSONPath = iota
	// ServiceSubFS The SubdirFS instance package.json path is valid in.
	ServiceSubFS = iota
	// The pointer to the Process instance of the service
	ServiceProc = iota
)

const (
	javaJarFlag      = "-jar"
	javaJarExtension = ".jar"
	dllExtension     = ".dll"
	javaApachePrefix = "org.apache."
	maxParseFileSize = 1024 * 1024
)

// ServiceMetadata holds information about a service.
type ServiceMetadata struct {
	Name              string
	AdditionalNames   []string
	DDService         string
	DDServiceInjected bool
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

// SetAdditionalNames set additional names for the service
func (s *ServiceMetadata) SetAdditionalNames(additional ...string) {
	if len(additional) > 1 {
		// names are discovered in unpredictable order. We need to keep them sorted if we're going to join them
		slices.Sort(additional)
	}
	s.AdditionalNames = additional
}

// SetNames sets generated names for the service.
func (s *ServiceMetadata) SetNames(name string, additional ...string) {
	s.Name = name
	s.SetAdditionalNames(additional...)
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

type simpleDetector struct {
	ctx DetectionContext
}

type dotnetDetector struct {
	ctx DetectionContext
}

func newSimpleDetector(ctx DetectionContext) detector {
	return &simpleDetector{ctx: ctx}
}

func newDotnetDetector(ctx DetectionContext) detector {
	return &dotnetDetector{ctx: ctx}
}

// DetectionContext allows to detect ServiceMetadata.
type DetectionContext struct {
	// Pid process PID
	Pid int
	// Args the command line arguments of the process
	Args []string
	// Envs targeted environment variables of the process
	Envs envs.Variables
	// Fs provides access to a file system
	fs fs.SubFS
	// DetectorContextMap a map to pass data between detectors, like some paths.
	ContextMap DetectorContextMap
}

// NewDetectionContext initializes DetectionContext.
func NewDetectionContext(args []string, envs envs.Variables, fs fs.SubFS) DetectionContext {
	return DetectionContext{
		Pid:  0,
		Args: args,
		Envs: envs,
		fs:   fs,
	}
}

// workingDirFromEnvs returns the current working dir extracted from the PWD env
func workingDirFromEnvs(envs envs.Variables) (string, bool) {
	return extractEnvVar(envs, "PWD")
}

func extractEnvVar(envs envs.Variables, name string) (string, bool) {
	value, ok := envs.Get(name)
	if !ok {
		return "", false
	}
	return value, len(value) > 0
}

// abs returns the path itself if already absolute or the absolute path by joining cwd with path
func abs(p string, cwd string) string {
	if path.IsAbs(p) || len(cwd) == 0 {
		return p
	}
	return path.Join(cwd, p)
}

// SizeVerifiedReader returns a reader for the file after ensuring that the file
// is a regular file and that the size that can be read from the reader will not
// exceed a pre-defined safety limit to control memory usage.
func SizeVerifiedReader(file fs.File) (io.Reader, error) {
	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}

	// Don't try to read device files, etc.
	if !fi.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}

	size := fi.Size()
	if size > maxParseFileSize {
		return nil, fmt.Errorf("file too large (%d bytes)", size)
	}

	// Additional limit the reader to avoid suprises if the file size changes
	// while reading it.
	return io.LimitReader(file, min(size, maxParseFileSize)), nil
}

// Map languages to their context detectors
var languageDetectors = map[language.Language]detectorCreatorFn{
	language.Python: newPythonDetector,
	language.Ruby:   newSimpleDetector,
	language.Java:   newJavaDetector,
	language.Node:   newNodeDetector,
	language.DotNet: newDotnetDetector,
	language.PHP:    newPhpDetector,
}

// Map executables that usually have additional process context of what's
// running, to context detectors
var executableDetectors = map[string]detectorCreatorFn{
	"gunicorn": newGunicornDetector,
	"puma":     newRailsDetector,
	"sudo":     newSimpleDetector,
}

func serviceNameInjected(envs envs.Variables) bool {
	if env, ok := envs.Get("DD_INJECTION_ENABLED"); ok {
		values := strings.Split(env, ",")
		for _, v := range values {
			if v == "service_name" {
				return true
			}
		}
	}
	return false
}

// ExtractServiceMetadata attempts to detect ServiceMetadata from the given process.
func ExtractServiceMetadata(lang language.Language, ctx DetectionContext) (metadata ServiceMetadata, success bool) {
	cmd := ctx.Args
	if len(cmd) == 0 || len(cmd[0]) == 0 {
		return
	}

	// We always return a service name from here on
	success = true

	if value, ok := chooseServiceNameFromEnvs(ctx.Envs); ok {
		metadata.DDService = value
		metadata.DDServiceInjected = serviceNameInjected(ctx.Envs)
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

	exe = normalizeExeName(exe)

	detectorProvider, ok := executableDetectors[exe]
	if !ok {
		detectorProvider, ok = languageDetectors[lang]
	}

	if ok {
		langMeta, ok := detectorProvider(ctx).detect(cmd[1:])

		// The detector could return a DD Service name (eg. Java, from the
		// dd.service property), but still fail to generate a service name (ok =
		// false) so check this first.
		if langMeta.DDService != "" {
			metadata.DDService = langMeta.DDService
		}

		if ok {
			metadata.Name = langMeta.Name
			metadata.SetAdditionalNames(langMeta.AdditionalNames...)
			return
		}
	}

	// trim trailing file extensions
	if i := strings.LastIndex(exe, "."); i > 0 {
		exe = exe[:i]
	}

	metadata.Name = exe
	return
}

func removeFilePath(s string) string {
	if s != "" {
		return path.Base(filepath.ToSlash(s))
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

func validVersion(str string) bool {
	if len(str) == 0 {
		return true
	}
	parts := strings.Split(str, ".")
	for _, v := range parts {
		for _, c := range v {
			if !unicode.IsNumber(c) {
				return false
			}
		}
	}
	return true
}

func normalizeExeName(exe string) string {
	// PHP Executable with version number - phpX.X
	if strings.HasPrefix(exe, "php") {
		suffix := exe[3:]
		if validVersion(suffix) {
			return "php"
		}
	}
	return exe
}

// chooseServiceNameFromEnvs extracts the service name from usual tracer env variables (DD_SERVICE, DD_TAGS).
// returns the service name, true if found, otherwise "", false
func chooseServiceNameFromEnvs(envs envs.Variables) (string, bool) {
	if val, ok := envs.Get("DD_SERVICE"); ok {
		return val, true
	}
	if val, ok := envs.Get("DD_TAGS"); ok && strings.Contains(val, "service:") {
		parts := strings.Split(val, ",")
		for _, p := range parts {
			if strings.HasPrefix(p, "service:") {
				return strings.TrimPrefix(p, "service:"), true
			}
		}
	}

	return "", false
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

func (dd dotnetDetector) detect(args []string) (ServiceMetadata, bool) {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		// when running assembly's dll, the cli must be executed without command
		// https://learn.microsoft.com/en-us/dotnet/core/tools/dotnet-run#description
		if strings.HasSuffix(strings.ToLower(a), dllExtension) {
			file := removeFilePath(a)
			return NewServiceMetadata(file[:len(file)-len(dllExtension)]), true
		}
		// dotnet cli syntax is something like `dotnet <cmd> <args> <dll> <prog args>`
		// if the first non arg (`-v, --something, ...) is not a dll file, exit early since nothing is matching a dll execute case
		break
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

// SubDirFS is like the fs.FS implemented by os.DirFS, except that it allows
// absolute paths to be passed in the Open/Stat/etc, and attaches them to the
// root dir.  It also implements SubFS, unlink the one implemented by os.DirFS.
type SubDirFS struct {
	fs.FS
	root string
}

// NewSubDirFS creates a new SubDirFS rooted at the specified path.
func NewSubDirFS(root string) SubDirFS {
	return SubDirFS{FS: os.DirFS(root), root: root}
}

// fixPath ensures that the specified path is stripped of the leading slash (if
// any) and is cleaned so that it can be passed to normal fs.FS functions which
// do not allow absolute paths or paths which do not pass fs.ValidPath (which
// contain ./ or .. for example).
func (s SubDirFS) fixPath(path string) string {
	path = filepath.Clean(path)
	if filepath.IsAbs(path) {
		return path[1:]
	}

	return path
}

// Readlink reads the specified symlink.
func (s SubDirFS) Readlink(name string) (string, error) {
	name = s.fixPath(name)
	return os.Readlink(filepath.Join(s.root, name))
}

// ReadlinkFS reads the symlink on the provided FS. There is no standard
// SymlinkFS interface yet https://github.com/golang/go/issues/49580.
func ReadlinkFS(fsfs fs.FS, name string) (string, error) {
	if subDirFS, ok := fsfs.(SubDirFS); ok {
		return subDirFS.Readlink(name)
	}

	return "", fs.ErrInvalid
}

// Sub provides a fs.FS for the specified subdirectory.
func (s SubDirFS) Sub(dir string) (fs.FS, error) {
	dir = filepath.Join(s.root, s.fixPath(dir))
	return os.DirFS(dir), nil
}

// ReadDir reads the specified subdirectory
func (s SubDirFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if readDirFS, ok := s.FS.(fs.ReadDirFS); ok {
		name = s.fixPath(name)
		return readDirFS.ReadDir(name)
	}

	return nil, fs.ErrInvalid
}

// Stat stats the specified file
func (s SubDirFS) Stat(name string) (fs.FileInfo, error) {
	if statFS, ok := s.FS.(fs.StatFS); ok {
		name = s.fixPath(name)
		return statFS.Stat(name)
	}

	return nil, fs.ErrInvalid
}

// Open opens the specified file
func (s SubDirFS) Open(name string) (fs.File, error) {
	return s.FS.Open(s.fixPath(name))
}

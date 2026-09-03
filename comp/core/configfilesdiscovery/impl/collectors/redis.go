// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/agent-payload/v5/agentdiscovery"
	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	RedisIntegrationName     = "redisdb"
	redisConfigPayloadFormat = agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_REDIS_CONF
	redisMaxIncludeDepth     = 10
	redisMaxConfigFiles      = 32
	redisMaxIncludeSearches  = 128
	redisMaxAggregateBytes   = 4 * 1024 * 1024
)

var redisDefaultConfigPaths = []string{
	"/etc/redis/redis.conf",
	"/usr/local/etc/redis/redis.conf",
	"/opt/bitnami/redis/etc/redis.conf",
}

type redisConfigCollector struct{}

// redisIncludeTraversal owns the state needed to collect one bounded Redis
// include tree.
type redisIncludeTraversal struct {
	ctx             context.Context
	reader          configfilesdiscoveryimpl.ConfigReader
	workingDir      string
	rootDir         string
	files           []configfilesdiscoveryimpl.ConfigFile
	visited         map[configfilesdiscoveryimpl.VerifiedConfigFilePath]struct{}
	totalBytes      int
	includeSearches int
	limited         bool
}

var redisEnvAllow = regexp.MustCompile(`^REDIS_[A-Z0-9_]+$`)

// Deny auth directives, arbitrary option bags, and connection strings that can
// contain inline credentials.
var redisEnvDeny = regexp.MustCompile(
	`^REDIS(_[A-Z0-9]+)*_(` +
		`AUTH|REQUIREPASS|MASTERAUTH|` +
		`ARGS|OPTS|FLAGS|` +
		`URL|URI|DSN|CONNECTION_STRING` +
		`)$`,
)

func NewRedis() configfilesdiscoveryimpl.ConfigCollector {
	return redisConfigCollector{}
}

// CanCollectFromProcess returns whether the command line contains an explicit,
// resolvable Redis config path.
func (redisConfigCollector) CanCollectFromProcess(commandline configfilesdiscoveryimpl.TargetCommandline) bool {
	configArg, ok := redisGetConfigArgFromCommandline(commandline.Args)
	if !ok {
		return false
	}
	_, resolved := resolveConfigPath(configArg, commandline.WorkingDir)
	return resolved
}

func (c redisConfigCollector) Collect(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) (configfilesdiscoveryimpl.CollectedConfig, error) {
	envVars, envErr := readEnvVars(ctx, reader, includeRedisEnvVar)
	if envErr != nil {
		log.Debugf("config files discovery skipped redis env var collection: %v", envErr)
		envVars = nil
	}

	envConfigPath := ""
	for _, envVar := range envVars {
		if envVar.Name == "REDIS_CONF_FILE" {
			envConfigPath = envVar.Value
			break
		}
	}

	selection, err := selectConfigFile(
		ctx,
		reader,
		redisGetConfigArgFromCommandline,
		redisMatchesCommandline,
		envConfigPath,
		redisDefaultConfigPaths,
	)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("collect redis config file: %w", err)
	}
	if selection == nil {
		// Without a config file, env vars are the only Redis config source.
		// Return the error so the scheduler retries.
		if envErr != nil {
			return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read redis env vars: %w", envErr)
		}
		if len(envVars) == 0 {
			log.Debugf("config files discovery skipped redis config collection: no config file or selected env vars detected")
			return configfilesdiscoveryimpl.CollectedConfig{}, nil
		}

		log.Debugf("config files discovery collected redis env vars without a config file")
		return configfilesdiscoveryimpl.CollectedConfig{
			EnvVars: envVars,
		}, nil
	}

	files, limited := collectRedisConfigFiles(ctx, reader, selection.file, selection.path, selection.workingDir)
	if limited {
		log.Warnf("config files discovery collected a partial redis config include tree rooted at %q because a safety limit was reached", selection.file.Path)
	}

	return configfilesdiscoveryimpl.CollectedConfig{
		ConfigFiles: files,
		EnvVars:     envVars,
	}, nil
}

// collectRedisConfigFiles returns the root followed by safe included files in
// Redis processing order. It also reports whether a safety limit made the
// result partial.
func collectRedisConfigFiles(
	ctx context.Context,
	reader configfilesdiscoveryimpl.ConfigReader,
	root configfilesdiscoveryimpl.ConfigFile,
	rootPath configfilesdiscoveryimpl.VerifiedConfigFilePath,
	workingDir string,
) ([]configfilesdiscoveryimpl.ConfigFile, bool) {
	root.Path = rootPath.String()
	root.PayloadFormat = redisConfigPayloadFormat

	traversal := redisIncludeTraversal{
		ctx:        ctx,
		reader:     reader,
		workingDir: workingDir,
		rootDir:    path.Dir(rootPath.String()),
		files:      []configfilesdiscoveryimpl.ConfigFile{root},
		visited:    map[configfilesdiscoveryimpl.VerifiedConfigFilePath]struct{}{rootPath: {}},
		totalBytes: len(root.Content),
	}
	traversal.collectFileIncludes(root, 1)
	return traversal.files, traversal.limited
}

// collectFileIncludes parses and collects the include directives in file.
func (t *redisIncludeTraversal) collectFileIncludes(file configfilesdiscoveryimpl.ConfigFile, depth int) {
	includes, parserOK := redisIncludePatterns(file.Content)
	if !parserOK {
		log.Debugf("config files discovery skipped malformed redis include directives in %q", file.Path)
	}
	for _, include := range includes {
		if depth >= redisMaxIncludeDepth ||
			len(t.files) >= redisMaxConfigFiles ||
			t.totalBytes >= redisMaxAggregateBytes ||
			t.includeSearches >= redisMaxIncludeSearches {
			t.limited = true
			return
		}
		t.collectInclude(include, file.Path, depth)
	}
}

// collectInclude resolves one include directive and collects its matching files.
func (t *redisIncludeTraversal) collectInclude(include string, parentPath string, depth int) {
	pattern, err := resolveRedisIncludePattern(include, t.workingDir, t.rootDir)
	if err != nil {
		log.Debugf("config files discovery skipped unsafe or unresolved redis include %q from %q: %v", include, parentPath, err)
		return
	}

	matchesPath, err := compileRedisIncludePattern(pattern)
	if err != nil {
		log.Debugf("config files discovery skipped malformed redis include %q from %q: %v", include, parentPath, err)
		return
	}

	matchesUnvisitedPath := func(filePath configfilesdiscoveryimpl.VerifiedConfigFilePath) (bool, error) {
		matched, err := matchesPath(filePath)
		if err != nil || !matched {
			return matched, err
		}
		if !isRedisPathWithinRoot(filePath.String(), t.rootDir) {
			log.Debugf("config files discovery skipped redis include match %q outside root directory %q", filePath.String(), t.rootDir)
			return false, nil
		}
		_, visited := t.visited[filePath]
		return !visited, nil
	}

	t.includeSearches++
	matches, matchesLimited, err := t.reader.FindFiles(t.ctx, pattern, redisMaxConfigFiles-len(t.files), matchesUnvisitedPath)
	if err != nil {
		log.Debugf("config files discovery skipped redis include %q from %q: %v", include, parentPath, err)
		return
	}
	if matchesLimited {
		t.limited = true
	}
	if len(matches) == 0 {
		log.Debugf("config files discovery found no files for redis include %q from %q", include, parentPath)
	}
	t.collectMatchingFiles(matches, depth)
}

// collectMatchingFiles reads and traverses matched include paths in order.
func (t *redisIncludeTraversal) collectMatchingFiles(matches []configfilesdiscoveryimpl.VerifiedConfigFilePath, depth int) {
	for _, match := range matches {
		if len(t.files) >= redisMaxConfigFiles || t.totalBytes >= redisMaxAggregateBytes {
			t.limited = true
			return
		}
		if _, found := t.visited[match]; found {
			continue
		}
		t.visited[match] = struct{}{}

		includedFile, err := t.reader.ReadFile(t.ctx, match)
		if err != nil {
			log.Debugf("config files discovery skipped unreadable redis include %q: %v", match.String(), err)
			continue
		}
		includedFile.Path = match.String()
		includedFile.PayloadFormat = redisConfigPayloadFormat
		remainingBytes := redisMaxAggregateBytes - t.totalBytes
		if len(includedFile.Content) > remainingBytes {
			includedFile.Content = includedFile.Content[:remainingBytes]
			includedFile.Truncated = true
			t.limited = true
		}

		t.files = append(t.files, includedFile)
		t.totalBytes += len(includedFile.Content)
		t.collectFileIncludes(includedFile, depth+1)
	}
}

// resolveRedisIncludePattern resolves a relative include against workingDir.
// It returns a cleaned pattern confined to rootDir.
func resolveRedisIncludePattern(include string, workingDir string, rootDir string) (configfilesdiscoveryimpl.VerifiedConfigFilePattern, error) {
	if include == "" || strings.Contains(include, "**") {
		return configfilesdiscoveryimpl.VerifiedConfigFilePattern{}, fmt.Errorf("unsupported include pattern %q", include)
	}
	rawPattern := include
	if !path.IsAbs(rawPattern) {
		if !path.IsAbs(workingDir) {
			return configfilesdiscoveryimpl.VerifiedConfigFilePattern{}, fmt.Errorf("relative include %q has no reliable working directory", include)
		}
		rawPattern = strings.TrimSuffix(workingDir, "/") + "/" + include
	}
	pattern, err := configfilesdiscoveryimpl.VerifyConfigFilePattern(configfilesdiscoveryimpl.UnverifiedConfigFilePattern(rawPattern))
	if err != nil {
		return configfilesdiscoveryimpl.VerifiedConfigFilePattern{}, err
	}
	if !isRedisPathWithinRoot(pattern.String(), rootDir) {
		return configfilesdiscoveryimpl.VerifiedConfigFilePattern{}, fmt.Errorf("include pattern %q is outside root directory %q", pattern.String(), rootDir)
	}
	return pattern, nil
}

// isRedisPathWithinRoot returns whether the cleaned absolute path is rootDir or
// one of its descendants, respecting path-component boundaries.
func isRedisPathWithinRoot(filePath string, rootDir string) bool {
	return rootDir == "/" || filePath == rootDir || strings.HasPrefix(filePath, rootDir+"/")
}

// compileRedisIncludePattern returns a matcher that implements Redis glob
// semantics, or an error when pattern is malformed.
func compileRedisIncludePattern(pattern configfilesdiscoveryimpl.VerifiedConfigFilePattern) (configfilesdiscoveryimpl.ConfigFilePathMatcher, error) {
	patternValue := pattern.String()
	convertedPattern := convertRedisIncludePattern(patternValue)
	if _, err := path.Match(convertedPattern, patternValue); err != nil {
		return nil, fmt.Errorf("invalid include pattern %q: %w", patternValue, err)
	}

	patternParts := strings.Split(patternValue, "/")
	return func(filePath configfilesdiscoveryimpl.VerifiedConfigFilePath) (bool, error) {
		filePathValue := filePath.String()
		pathParts := strings.Split(filePathValue, "/")
		if len(patternParts) == len(pathParts) {
			for i, pathPart := range pathParts {
				if strings.HasPrefix(pathPart, ".") && !startsWithExplicitPeriod(patternParts[i]) {
					return false, nil
				}
			}
		}

		matched, _ := path.Match(convertedPattern, filePathValue)
		return matched, nil
	}, nil
}

// startsWithExplicitPeriod returns whether a pattern component explicitly
// names a leading period.
func startsWithExplicitPeriod(patternPart string) bool {
	return strings.HasPrefix(patternPart, ".") || strings.HasPrefix(patternPart, `\.`)
}

// convertRedisIncludePattern converts Redis glob syntax into syntax accepted by
// path.Match.
func convertRedisIncludePattern(pattern string) string {
	var converted strings.Builder
	escaped := false
	for i := 0; i < len(pattern); i++ {
		char := pattern[i]
		if escaped {
			converted.WriteByte(char)
			escaped = false
			continue
		}
		if char == '\\' {
			converted.WriteByte(char)
			escaped = true
			continue
		}
		if char == '[' && i+1 < len(pattern) && pattern[i+1] == '!' {
			converted.WriteString("[^")
			i++
			continue
		}
		converted.WriteByte(char)
	}
	return converted.String()
}

// redisIncludePatterns returns valid Redis include arguments in source order.
// It also reports whether tokenization completed without malformed lines or
// scanner errors.
func redisIncludePatterns(content []byte) ([]string, bool) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 1024), 1024*1024+1)
	var includes []string
	parserOK := true
	for scanner.Scan() {
		args, ok := splitRedisConfigLine(scanner.Text())
		if !ok {
			parserOK = false
			continue
		}
		if len(args) == 2 && strings.EqualFold(args[0], "include") {
			includes = append(includes, args[1])
		}
	}
	return includes, parserOK && scanner.Err() == nil
}

// splitRedisConfigLine tokenizes a Redis configuration line and reports whether
// its quoting and token boundaries are well formed.
func splitRedisConfigLine(line string) ([]string, bool) {
	var args []string
	for pos := 0; ; {
		for pos < len(line) && isRedisConfigSpace(line[pos]) {
			pos++
		}
		if pos == len(line) {
			return args, true
		}
		if len(args) == 0 && line[pos] == '#' {
			return args, true
		}

		arg, next, ok := readRedisConfigArgument(line, pos)
		if !ok {
			return nil, false
		}
		args = append(args, arg)
		pos = next
		if pos < len(line) && !isRedisConfigSpace(line[pos]) {
			return nil, false
		}
	}
}

// readRedisConfigArgument reads one Redis configuration argument starting at
// start. It returns the decoded value, the next input position, and whether the
// argument is well formed.
func readRedisConfigArgument(line string, start int) (string, int, bool) {
	var value strings.Builder
	doubleQuoted := false
	singleQuoted := false
	for pos := start; pos < len(line); pos++ {
		current := line[pos]
		if !doubleQuoted && !singleQuoted {
			switch current {
			case '"':
				doubleQuoted = true
				continue
			case '\'':
				singleQuoted = true
				continue
			default:
				if isRedisConfigSpace(current) {
					return value.String(), pos, true
				}
				value.WriteByte(current)
				continue
			}
		}

		if singleQuoted {
			if current == '\'' {
				return value.String(), pos + 1, true
			}
			if current == '\\' && pos+1 < len(line) && line[pos+1] == '\'' {
				value.WriteByte('\'')
				pos++
				continue
			}
			value.WriteByte(current)
			continue
		}

		if current == '"' {
			return value.String(), pos + 1, true
		}
		if current != '\\' {
			value.WriteByte(current)
			continue
		}
		if pos+1 >= len(line) {
			return "", 0, false
		}
		pos++
		escaped := line[pos]
		if escaped == 'x' && pos+2 < len(line) {
			hexValue, err := strconv.ParseUint(line[pos+1:pos+3], 16, 8)
			if err == nil {
				value.WriteByte(byte(hexValue))
				pos += 2
				continue
			}
		}
		switch escaped {
		case 'n':
			value.WriteByte('\n')
		case 'r':
			value.WriteByte('\r')
		case 't':
			value.WriteByte('\t')
		case 'b':
			value.WriteByte('\b')
		case 'a':
			value.WriteByte('\a')
		default:
			value.WriteByte(escaped)
		}
	}
	if doubleQuoted || singleQuoted {
		return "", 0, false
	}
	return value.String(), len(line), true
}

// isRedisConfigSpace returns whether value separates Redis configuration arguments.
func isRedisConfigSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r'
}

func includeRedisEnvVar(name string) bool {
	if configfilesdiscoveryimpl.IsSecretEnvVarName(name) || redisEnvDeny.MatchString(name) {
		return false
	}
	if name == "OPENSSL_FIPS" {
		return true
	}
	return redisEnvAllow.MatchString(name)
}

// redisGetConfigArgFromCommandline returns the explicit config argument passed
// to redis-server. Redis also accepts command-line options as temporary config,
// but those options do not identify a file this collector can read.
func redisGetConfigArgFromCommandline(args []string) (string, bool) {
	args = unwrapShellCommandline(args)
	redisArgs, ok := redisGetArgs(args)
	if !ok {
		return "", false
	}
	return redisGetConfigArg(redisArgs)
}

func redisMatchesCommandline(args []string) bool {
	_, ok := redisGetArgs(unwrapShellCommandline(args))
	return ok
}

func redisGetArgs(args []string) ([]string, bool) {
	for i, arg := range args {
		if path.Base(arg) == "redis-server" {
			return args[i+1:], true
		}
	}
	return nil, false
}

// redisGetConfigArg returns the positional config path that redis-server
// accepts before command-line options. A flags-only startup is intentionally
// skipped because it has no config file to read.
func redisGetConfigArg(redisArgs []string) (string, bool) {
	if len(redisArgs) == 0 || redisArgs[0] == "" || strings.HasPrefix(redisArgs[0], "-") {
		return "", false
	}
	return redisArgs[0], true
}

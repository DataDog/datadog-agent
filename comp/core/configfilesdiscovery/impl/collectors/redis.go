// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/DataDog/agent-payload/v5/agentdiscovery"
	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	RedisIntegrationName     = "redisdb"
	redisConfigPayloadFormat = agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_REDIS_CONF
)

type redisConfigCollector struct{}

var redisEnvAllow = []*regexp.Regexp{
	regexp.MustCompile(`^REDIS_[A-Z0-9_]+$`),
}

var redisEnvDeny = []*regexp.Regexp{
	// Option bags can contain arbitrary Redis arguments and inline credentials.
	regexp.MustCompile(`^REDIS(_[A-Z0-9]+)*_(ARGS|OPTS|FLAGS)$`),
	// Connection strings can contain usernames and passwords.
	regexp.MustCompile(`^REDIS(_[A-Z0-9]+)*_(URL|URI|DSN|CONNECTION_STRING)$`),
}

func NewRedis() configfilesdiscoveryimpl.ConfigCollector {
	return redisConfigCollector{}
}

func (c redisConfigCollector) Collect(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) (configfilesdiscoveryimpl.CollectedConfig, error) {
	commandline, err := reader.ReadCommandline(ctx)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read redis command line: %w", err)
	}

	env, envErr := reader.ReadEnvVars(ctx, includeRedisEnvVar)
	if envErr != nil {
		log.Debugf("config files discovery skipped redis env var collection: %v", envErr)
		env = nil
	}
	envVars := redisBuildEnvVars(env)

	configPath, ok := redisGetConfigPath(commandline)
	if !ok {
		configPath, ok = resolveConfigPath(env["REDIS_CONF_FILE"], commandline.WorkingDir)
	}
	if !ok {
		// Without a config file path, env vars are the only Redis config source.
		// Return the error so the scheduler retries.
		if envErr != nil {
			return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read redis env vars: %w", envErr)
		}
		if len(envVars) == 0 {
			log.Debugf("config files discovery skipped redis config collection: no explicit config file path or selected env vars detected")
			return configfilesdiscoveryimpl.CollectedConfig{}, nil
		}

		log.Debugf("config files discovery collected redis env vars without an explicit config file path")
		return configfilesdiscoveryimpl.CollectedConfig{
			EnvVars: envVars,
		}, nil
	}

	file, err := reader.ReadFile(ctx, configPath)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read redis config file %q: %w", configPath, err)
	}
	file.PayloadFormat = redisConfigPayloadFormat

	return configfilesdiscoveryimpl.CollectedConfig{
		ConfigFiles: []configfilesdiscoveryimpl.ConfigFile{file},
		EnvVars:     envVars,
	}, nil
}

func redisBuildEnvVars(env map[string]string) []configfilesdiscoveryimpl.ConfigEnvVar {
	if len(env) == 0 {
		return nil
	}

	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	sort.Strings(names)

	envVars := make([]configfilesdiscoveryimpl.ConfigEnvVar, 0, len(env))
	for _, name := range names {
		envVars = append(envVars, configfilesdiscoveryimpl.ConfigEnvVar{
			Name:  name,
			Value: env[name],
		})
	}
	return envVars
}

func includeRedisEnvVar(name string) bool {
	if configfilesdiscoveryimpl.IsSecretEnvVarName(name) || denyRedisEnvVar(name) {
		return false
	}
	if name == "OPENSSL_FIPS" {
		return true
	}
	for _, re := range redisEnvAllow {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

func denyRedisEnvVar(name string) bool {
	for _, re := range redisEnvDeny {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

// redisGetConfigPath returns the explicit config file path passed to
// redis-server. Redis also accepts command-line options as temporary config,
// but those options do not identify a file this collector can read.
func redisGetConfigPath(commandline configfilesdiscoveryimpl.TargetCommandline) (string, bool) {
	args := commandlineArgs(commandline)
	redisArgs, ok := redisGetArgs(args)
	if !ok {
		return "", false
	}

	configPath, ok := redisGetConfigArg(redisArgs)
	if !ok {
		return "", false
	}
	return resolveConfigPath(configPath, commandline.WorkingDir)
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

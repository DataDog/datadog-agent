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
	"strings"

	"github.com/DataDog/agent-payload/v5/agentdiscovery"
	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	RedisIntegrationName     = "redisdb"
	redisConfigPayloadFormat = agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_REDIS_CONF
)

var redisDefaultConfigPaths = []string{
	"/etc/redis/redis.conf",
	"/usr/local/etc/redis/redis.conf",
	"/opt/bitnami/redis/etc/redis.conf",
}

type redisConfigCollector struct{}

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

	file, ok, err := readConfigFile(
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
	if !ok {
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

	file.PayloadFormat = redisConfigPayloadFormat

	return configfilesdiscoveryimpl.CollectedConfig{
		ConfigFiles: []configfilesdiscoveryimpl.ConfigFile{file},
		EnvVars:     envVars,
	}, nil
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

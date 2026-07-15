// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"fmt"
	"path"
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

func NewRedis() configfilesdiscoveryimpl.ConfigCollector {
	return redisConfigCollector{}
}

func (c redisConfigCollector) Collect(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) ([]configfilesdiscoveryimpl.ConfigFile, error) {
	commandline, err := reader.ReadCommandline(ctx)
	if err != nil {
		return nil, fmt.Errorf("read redis command line: %w", err)
	}

	configPath, ok := redisGetConfigPath(commandline)
	if !ok {
		log.Debugf("config files discovery skipped redis config collection: no explicit config file path detected")
		return nil, nil
	}

	file, err := reader.ReadFile(ctx, configPath)
	if err != nil {
		return nil, fmt.Errorf("read redis config file %q: %w", configPath, err)
	}
	file.PayloadFormat = redisConfigPayloadFormat

	return []configfilesdiscoveryimpl.ConfigFile{file}, nil
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

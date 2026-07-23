// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"fmt"
	"path"

	"github.com/DataDog/agent-payload/v5/agentdiscovery"
	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	KafkaIntegrationName     = "kafka"
	kafkaConfigPayloadFormat = agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_PROPERTIES
)

type kafkaConfigCollector struct{}

func NewKafka() configfilesdiscoveryimpl.ConfigCollector {
	return kafkaConfigCollector{}
}

// MatchesCommandline returns whether the command line contains an explicit
// Kafka broker properties argument. Path resolution is deferred until
// collection because workloadmeta process events may not include the working
// directory.
func (kafkaConfigCollector) MatchesCommandline(args []string) bool {
	_, ok := kafkaGetConfigArgFromCommandline(args)
	return ok
}

func (c kafkaConfigCollector) Collect(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) (configfilesdiscoveryimpl.CollectedConfig, error) {
	configPath, ok, err := findConfigPath(ctx, reader, kafkaGetConfigArgFromCommandline)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read kafka command lines: %w", err)
	}
	if !ok {
		log.Debugf("config files discovery skipped kafka config collection: no explicit broker properties file path detected")
		return configfilesdiscoveryimpl.CollectedConfig{}, nil
	}

	file, err := reader.ReadFile(ctx, configPath)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read kafka config file %q: %w", configPath, err)
	}
	file.PayloadFormat = kafkaConfigPayloadFormat

	return configfilesdiscoveryimpl.CollectedConfig{
		ConfigFiles: []configfilesdiscoveryimpl.ConfigFile{file},
	}, nil
}

// kafkaGetConfigArgFromCommandline returns the broker properties argument
// passed to the Kafka server launcher. It intentionally ignores command-line
// --override values: those mutate runtime config but do not identify an
// additional file to read.
func kafkaGetConfigArgFromCommandline(args []string) (string, bool) {
	args = unwrapShellCommandline(args)
	kafkaArgs, ok := kafkaGetArgs(args)
	if !ok {
		return "", false
	}
	return kafkaGetConfigArg(kafkaArgs)
}

func kafkaGetArgs(args []string) ([]string, bool) {
	for i, arg := range args {
		switch path.Base(arg) {
		case "kafka-server-start.sh", "kafka-server-start", "kafka.Kafka":
			return args[i+1:], true
		}
	}
	return nil, false
}

func kafkaGetConfigArg(kafkaArgs []string) (string, bool) {
	for i := 0; i < len(kafkaArgs); i++ {
		arg := kafkaArgs[i]
		switch {
		case arg == "":
			return "", false
		case arg == "-daemon":
			continue
		case arg == "--override":
			i++
			continue
		case hasKafkaInlineOverrideArg(arg):
			continue
		case arg[0] == '-':
			return "", false
		default:
			return arg, true
		}
	}
	return "", false
}

func hasKafkaInlineOverrideArg(arg string) bool {
	const prefix = "--override="
	return len(arg) > len(prefix) && arg[:len(prefix)] == prefix
}

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

	"github.com/DataDog/agent-payload/v5/agentdiscovery"
	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	KafkaIntegrationName     = "kafka"
	kafkaConfigPayloadFormat = agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_PROPERTIES
)

// kafkaDefaultConfigPathGroups contains final config paths passed to Kafka by
// known distribution image startup scripts, ordered by priority. Strimzi's
// generated config takes precedence over the packaged Apache config that its
// image also contains. Apache and Confluent images ship other example configs,
// but their startup scripts do not pass those files to Kafka.
var kafkaDefaultConfigPathGroups = [][]string{
	{"/tmp/strimzi.properties"},
	{
		"/opt/kafka/config/server.properties",
		"/etc/kafka/kafka.properties",
		"/opt/bitnami/kafka/config/server.properties",
	},
}

type kafkaConfigCollector struct{}

var kafkaEnvAllow = []*regexp.Regexp{
	regexp.MustCompile(`^KAFKA_[A-Z0-9_]+$`),
	regexp.MustCompile(`^CONFLUENT_[A-Z0-9_]+$`),
}

var kafkaEnvDeny = []*regexp.Regexp{
	// Option and command bags can contain arbitrary JVM/broker args, shell code,
	// and inline credentials.
	// Leave collection of safe sub-parts to a future, explicit design.
	regexp.MustCompile(`^(KAFKA|CONFLUENT)(_[A-Z0-9]+)*_(OPTS|COMMAND|EXTRA_(ARGS|FLAGS))$`),
	// Confluent basic.auth.user.info values are username:password pairs.
	regexp.MustCompile(`^(KAFKA|CONFLUENT)(_[A-Z0-9]+)*_BASIC_AUTH_USER_INFO$`),
	regexp.MustCompile(`^(KAFKA|KAFKA_CFG)_SUPER_USERS$`),
}

func NewKafka() configfilesdiscoveryimpl.ConfigCollector {
	return kafkaConfigCollector{}
}

// CanCollectFromProcess returns whether the command line contains an explicit,
// resolvable Kafka broker properties path.
func (kafkaConfigCollector) CanCollectFromProcess(commandline configfilesdiscoveryimpl.TargetCommandline) bool {
	configArg, ok := kafkaGetConfigArgFromCommandline(commandline.Args)
	if !ok {
		return false
	}
	_, resolved := resolveConfigPath(configArg, commandline.WorkingDir)
	return resolved
}

func (c kafkaConfigCollector) Collect(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) (configfilesdiscoveryimpl.CollectedConfig, error) {
	file, ok, err := readConfigFile(ctx, reader, kafkaGetConfigArgFromCommandline, kafkaMatchesCommandline, kafkaDefaultConfigPathGroups...)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("collect kafka config file: %w", err)
	}

	envVars, err := kafkaReadEnvVars(ctx, reader)
	if err != nil {
		log.Debugf("config files discovery skipped kafka env var collection: %v", err)
		envVars = nil
	}
	if !ok {
		// Without a broker properties file, env vars are the only
		// Kafka config source. Return the error so the scheduler retries.
		if err != nil {
			return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read kafka env vars: %w", err)
		}
		if len(envVars) == 0 {
			log.Debugf("config files discovery skipped kafka config collection: no broker properties file or selected env vars detected")
			return configfilesdiscoveryimpl.CollectedConfig{}, nil
		}

		log.Debugf("config files discovery collected kafka env vars without an explicit broker properties file path")
		return configfilesdiscoveryimpl.CollectedConfig{
			EnvVars: envVars,
		}, nil
	}

	file.PayloadFormat = kafkaConfigPayloadFormat

	return configfilesdiscoveryimpl.CollectedConfig{
		ConfigFiles: []configfilesdiscoveryimpl.ConfigFile{file},
		EnvVars:     envVars,
	}, nil
}

func kafkaReadEnvVars(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) ([]configfilesdiscoveryimpl.ConfigEnvVar, error) {
	env, err := reader.ReadEnvVars(ctx, includeKafkaEnvVar)
	if err != nil {
		return nil, err
	}

	if len(env) == 0 {
		return nil, nil
	}

	envVars := make([]configfilesdiscoveryimpl.ConfigEnvVar, 0, len(env))
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		envVars = append(envVars, configfilesdiscoveryimpl.ConfigEnvVar{
			Name:  name,
			Value: env[name],
		})
	}

	return envVars, nil
}

func includeKafkaEnvVar(name string) bool {
	if denyKafkaEnvVar(name) {
		return false
	}
	if name == "CLUSTER_ID" {
		return true
	}
	for _, re := range kafkaEnvAllow {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

func denyKafkaEnvVar(name string) bool {
	for _, re := range kafkaEnvDeny {
		if re.MatchString(name) {
			return true
		}
	}
	return false
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

func kafkaMatchesCommandline(args []string) bool {
	_, ok := kafkaGetArgs(unwrapShellCommandline(args))
	return ok
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

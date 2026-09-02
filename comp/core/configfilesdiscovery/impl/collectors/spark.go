// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"fmt"
	"regexp"

	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
)

// SparkIntegrationName is the Autodiscovery check name for Apache Spark.
const SparkIntegrationName = "spark"

const sparkMasterMode = "master"

type sparkConfigCollector struct{}

// sparkEnvAllow accepts Spark's documented environment-variable namespace.
// Explicit denials below reject settings whose values may contain arbitrary
// arguments. The common reader policy rejects secret-shaped names first.
var sparkEnvAllow = regexp.MustCompile(`^SPARK_[A-Z0-9_]+$`)

var sparkEnvDeny = regexp.MustCompile(
	`^SPARK_(?:.*_)?(?:OPTS|JAVA_OPTS|JAVA_OPTIONS|CLASSPATH|COMMAND|EXTRA_(?:ARGS|FLAGS))$`,
)

// NewSpark returns a collector for Spark Standalone Master environment
// variables. Worker, Driver, and YARN collection are intentionally out of
// scope for this collector.
func NewSpark() configfilesdiscoveryimpl.ConfigCollector {
	return sparkConfigCollector{}
}

// CanCollectFromProcess always returns false: this collector only reads
// environment variables from an Autodiscovery target and has no process-based
// retry behavior.
func (sparkConfigCollector) CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline) bool {
	return false
}

func (sparkConfigCollector) Collect(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) (configfilesdiscoveryimpl.CollectedConfig, error) {
	envVars, err := readEnvVars(ctx, reader, includeSparkEnvVar)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read spark master env vars: %w", err)
	}

	isMaster, err := isSparkMaster(ctx, reader, envVars)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, err
	}
	if !isMaster {
		return configfilesdiscoveryimpl.CollectedConfig{}, nil
	}
	if len(envVars) == 0 {
		return configfilesdiscoveryimpl.CollectedConfig{}, nil
	}

	return configfilesdiscoveryimpl.CollectedConfig{EnvVars: envVars}, nil
}

func isSparkMaster(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader, envVars []configfilesdiscoveryimpl.ConfigEnvVar) (bool, error) {
	commandline, err := reader.ReadRuntimeCommandline(ctx)
	if err != nil {
		return false, fmt.Errorf("read spark runtime command line: %w", err)
	}
	if isSparkMasterCommand(commandline.Args) {
		return true, nil
	}

	for _, commandline := range reader.ReadLiveProcessCommandlines(ctx) {
		if isSparkMasterCommand(commandline.Args) {
			return true, nil
		}
	}

	// Bitnami's Docker entrypoint does not expose the Java Master class in the
	// container command line. Use its documented role variable only as a
	// fallback when the Agent cannot inspect a matching live process.
	for _, envVar := range envVars {
		if envVar.Name == "SPARK_MODE" {
			return envVar.Value == sparkMasterMode, nil
		}
	}
	return false, nil
}

func isSparkMasterCommand(args []string) bool {
	for _, arg := range args {
		if arg == "org.apache.spark.deploy.master.Master" {
			return true
		}
	}
	return false
}

func includeSparkEnvVar(name string) bool {
	return !configfilesdiscoveryimpl.IsSecretEnvVarName(name) &&
		sparkEnvAllow.MatchString(name) &&
		!sparkEnvDeny.MatchString(name)
}

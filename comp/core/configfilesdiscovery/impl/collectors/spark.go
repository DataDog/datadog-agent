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
	// SparkIntegrationName is the Autodiscovery check name for Spark.
	SparkIntegrationName        = "spark"
	sparkConfigPayloadFormat    = agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_PROPERTIES
	sparkDefaultsConfigFileName = "spark-defaults.conf"
	sparkSubmitClass            = "org.apache.spark.deploy.SparkSubmit"
	sparkPropertiesFileOption   = "--properties-file"

	// sparkStandaloneDriverClass is launched by a Spark Standalone Worker for
	// applications submitted in cluster deploy mode. It is the stable process
	// identity that distinguishes a Driver from the submitting client and the
	// Worker itself.
	sparkStandaloneDriverClass = "org.apache.spark.deploy.worker.DriverWrapper"
)

type sparkConfigCollector struct{}

// sparkEnvAllowlist contains the documented, non-secret environment variables
// supported by Apache Spark Standalone and the Bitnami Spark image. JVM options,
// commands, arbitrary extensions, and secret-bearing variables are deliberately
// excluded: their values are not safe to forward by name alone.
var sparkEnvAllowlist = map[string]struct{}{
	"SPARK_BASE_DIR":                         {},
	"SPARK_CONF_DIR":                         {},
	"SPARK_CONF_FILE":                        {},
	"SPARK_DAEMON_GROUP":                     {},
	"SPARK_DAEMON_MEMORY":                    {},
	"SPARK_DAEMON_USER":                      {},
	"SPARK_DEFAULT_CONF_DIR":                 {},
	"SPARK_DRIVER_CORES":                     {},
	"SPARK_DRIVER_MEMORY":                    {},
	"SPARK_EXECUTOR_CORES":                   {},
	"SPARK_EXECUTOR_MEMORY":                  {},
	"SPARK_INITSCRIPTS_DIR":                  {},
	"SPARK_IDENT_STRING":                     {},
	"SPARK_JARS_DIR":                         {},
	"SPARK_LOCAL_DIRS":                       {},
	"SPARK_LOCAL_IP":                         {},
	"SPARK_LOCAL_STORAGE_ENCRYPTION_ENABLED": {},
	"SPARK_LOG_DIR":                          {},
	"SPARK_LOG_MAX_FILES":                    {},
	"SPARK_MASTER_HOST":                      {},
	"SPARK_MASTER_PORT":                      {},
	"SPARK_MASTER_URL":                       {},
	"SPARK_MASTER_WEBUI_PORT":                {},
	"SPARK_METRICS_ENABLED":                  {},
	"SPARK_MODE":                             {},
	"SPARK_NICENESS":                         {},
	"SPARK_NO_DAEMONIZE":                     {},
	"SPARK_PID_DIR":                          {},
	"SPARK_PUBLIC_DNS":                       {},
	"SPARK_RPC_AUTHENTICATION_ENABLED":       {},
	"SPARK_RPC_ENCRYPTION_ENABLED":           {},
	"SPARK_SSL_ENABLED":                      {},
	"SPARK_SSL_KEYSTORE_FILE":                {},
	"SPARK_SSL_KEYSTORE_TYPE":                {},
	"SPARK_SSL_NEED_CLIENT_AUTH":             {},
	"SPARK_SSL_PROTOCOL":                     {},
	"SPARK_SSL_TRUSTSTORE_FILE":              {},
	"SPARK_SSL_TRUSTSTORE_TYPE":              {},
	"SPARK_TMP_DIR":                          {},
	"SPARK_USER":                             {},
	"SPARK_WEBUI_SSL_PORT":                   {},
	"SPARK_WORK_DIR":                         {},
	"SPARK_WORKER_CORES":                     {},
	"SPARK_WORKER_DIR":                       {},
	"SPARK_WORKER_MEMORY":                    {},
	"SPARK_WORKER_PORT":                      {},
	"SPARK_WORKER_WEBUI_PORT":                {},
}

var sparkDefaultConfigPathGroups = [][]string{
	{"/opt/spark/conf/" + sparkDefaultsConfigFileName},
	{"/opt/bitnami/spark/conf/" + sparkDefaultsConfigFileName},
}

// NewSpark returns a collector for Spark Standalone Drivers running in cluster
// deploy mode. It reads the Driver's Spark properties file and supporting
// non-secret environment metadata. Other Spark deployment modes do not have a
// portable, unambiguous Driver process identity available through ConfigReader.
func NewSpark() configfilesdiscoveryimpl.ConfigCollector {
	return sparkConfigCollector{}
}

// CanCollectFromProcess returns whether a process event identifies a Spark
// Standalone Driver and can trigger the one-shot recollection fallback.
func (sparkConfigCollector) CanCollectFromProcess(commandline configfilesdiscoveryimpl.TargetCommandline) bool {
	return isSparkDriverCommand(commandline.Args)
}

func (sparkConfigCollector) Collect(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) (configfilesdiscoveryimpl.CollectedConfig, error) {
	isDriver, err := isSparkDriver(ctx, reader)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("identify spark driver: %w", err)
	}
	if !isDriver {
		log.Debugf("config files discovery skipped spark driver env collection: no DriverWrapper process detected")
		return configfilesdiscoveryimpl.CollectedConfig{}, nil
	}

	envVars, envErr := readEnvVars(ctx, reader, includeSparkEnvVar)
	if envErr != nil {
		log.Debugf("config files discovery skipped spark driver env var collection: %v", envErr)
		envVars = nil
	}

	file, ok, err := readConfigFile(
		ctx,
		reader,
		sparkGetPropertiesFileFromCommandline,
		sparkMatchesSubmitCommandline,
		sparkFallbackConfigArg(envVars),
		sparkDefaultConfigPathGroups...,
	)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("collect spark driver config file: %w", err)
	}
	if !ok {
		if envErr != nil {
			return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read spark driver env vars: %w", envErr)
		}
		if len(envVars) == 0 {
			return configfilesdiscoveryimpl.CollectedConfig{}, nil
		}
		return configfilesdiscoveryimpl.CollectedConfig{EnvVars: envVars}, nil
	}

	file.PayloadFormat = sparkConfigPayloadFormat
	return configfilesdiscoveryimpl.CollectedConfig{
		ConfigFiles: []configfilesdiscoveryimpl.ConfigFile{file},
		EnvVars:     envVars,
	}, nil
}

// sparkFallbackConfigArg uses Spark's documented configuration-directory
// override. When it is absent, readConfigFile considers known image defaults.
func sparkFallbackConfigArg(envVars []configfilesdiscoveryimpl.ConfigEnvVar) string {
	for _, envVar := range envVars {
		if envVar.Name == "SPARK_CONF_DIR" && envVar.Value != "" {
			return path.Join(envVar.Value, sparkDefaultsConfigFileName)
		}
	}
	return ""
}

// sparkGetPropertiesFileFromCommandline returns the explicit properties file
// passed to SparkSubmit. An explicit path is authoritative over SPARK_CONF_DIR
// and image defaults.
func sparkGetPropertiesFileFromCommandline(args []string) (string, bool) {
	args = unwrapShellCommandline(args)
	for i, arg := range args {
		if arg != sparkSubmitClass && path.Base(arg) != "spark-submit" {
			continue
		}
		for i++; i < len(args); i++ {
			switch {
			case args[i] == sparkPropertiesFileOption && i+1 < len(args):
				return args[i+1], true
			case strings.HasPrefix(args[i], sparkPropertiesFileOption+"="):
				return strings.TrimPrefix(args[i], sparkPropertiesFileOption+"="), true
			}
		}
		return "", false
	}
	return "", false
}

func sparkMatchesSubmitCommandline(args []string) bool {
	args = unwrapShellCommandline(args)
	for _, arg := range args {
		if arg == sparkSubmitClass || path.Base(arg) == "spark-submit" {
			return true
		}
	}
	return false
}

func includeSparkEnvVar(name string) bool {
	_, allowed := sparkEnvAllowlist[name]
	return allowed && !configfilesdiscoveryimpl.IsSecretEnvVarName(name)
}

func isSparkDriver(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) (bool, error) {
	runtimeCommandline, runtimeErr := reader.ReadRuntimeCommandline(ctx)
	if runtimeErr == nil && isSparkDriverCommand(runtimeCommandline.Args) {
		return true, nil
	}

	for _, commandline := range reader.ReadLiveProcessCommandlines(ctx) {
		if isSparkDriverCommand(commandline.Args) {
			return true, nil
		}
	}

	return false, runtimeErr
}

func isSparkDriverCommand(args []string) bool {
	for _, arg := range unwrapShellCommandline(args) {
		if arg == sparkStandaloneDriverClass {
			return true
		}
	}
	return false
}

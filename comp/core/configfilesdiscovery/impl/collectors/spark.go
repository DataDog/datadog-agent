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
	sparkDriverRoleTag          = "spark_process_role:driver"
	sparkDDTagsPrefix           = "-Ddd.tags="

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

// Spark's Apache and Bitnami images keep the default properties file in
// different locations. The paths are individual priority groups so the
// collector selects the Apache path when both happen to be readable.
var sparkDefaultConfigPathGroups = [][]string{
	{"/opt/spark/conf/" + sparkDefaultsConfigFileName},
	{"/opt/bitnami/spark/conf/" + sparkDefaultsConfigFileName},
}

// NewSpark returns a collector for Spark drivers. It identifies Spark
// Standalone drivers by their DriverWrapper class and SparkSubmit drivers by
// their explicit spark_process_role:driver JVM tag. It reads the driver's Spark
// properties file in addition to supporting non-secret environment metadata.
func NewSpark() configfilesdiscoveryimpl.ConfigCollector {
	return sparkConfigCollector{}
}

// CanCollectFromProcess returns whether a process event identifies a Spark
// Standalone Driver and can trigger the one-shot recollection fallback. A
// SparkSubmit process needs the exact driver role tag to avoid collecting from
// submitting clients and other Spark roles.
func (sparkConfigCollector) CanCollectFromProcess(commandline configfilesdiscoveryimpl.TargetCommandline) bool {
	return isSparkDriverCommand(commandline.Args)
}

func (sparkConfigCollector) Collect(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) (configfilesdiscoveryimpl.CollectedConfig, error) {
	isDriver, err := isSparkDriver(ctx, reader)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("identify spark driver: %w", err)
	}
	if !isDriver {
		return configfilesdiscoveryimpl.CollectedConfig{}, nil
	}

	// Initialize result early and populate fields as we collect them
	result := configfilesdiscoveryimpl.CollectedConfig{}

	// Collect env vars (best-effort)
	envVars, envErr := readEnvVars(ctx, reader, includeSparkEnvVar)
	if envErr != nil {
		log.Debugf("config files discovery skipped spark driver env var collection: %v", envErr)
	} else {
		result.EnvVars = envVars
	}

	// Collect config file (best-effort)
	file, ok, fileErr := readSparkConfigFile(ctx, reader, envVars)
	if fileErr != nil {
		log.Debugf("config files discovery could not read spark driver config file: %v", fileErr)
	} else if ok {
		file.PayloadFormat = sparkConfigPayloadFormat
		result.ConfigFiles = []configfilesdiscoveryimpl.ConfigFile{file}
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, ctxErr
	}

	// Return what we collected
	if len(result.EnvVars) == 0 && len(result.ConfigFiles) == 0 {
		if envErr != nil {
			return result, fmt.Errorf("read spark driver env vars: %w", envErr)
		}
		if fileErr != nil {
			return result, fmt.Errorf("collect spark driver config file: %w", fileErr)
		}
		log.Debugf("config files discovery skipped spark driver config collection: no config file or selected env vars detected")
		return result, nil
	}

	return result, nil
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

// readSparkConfigFile preserves Spark's configuration precedence while keeping
// SPARK_CONF_DIR local to this collector. A properties file explicitly named
// by SparkSubmit is authoritative; SPARK_CONF_DIR is only an optional hint.
func readSparkConfigFile(
	ctx context.Context,
	reader configfilesdiscoveryimpl.ConfigReader,
	envVars []configfilesdiscoveryimpl.ConfigEnvVar,
) (configfilesdiscoveryimpl.ConfigFile, bool, error) {
	file, ok, explicitFound, runtimeWorkingDir, err := readSparkExplicitPropertiesFile(ctx, reader)
	if err != nil || ok || explicitFound {
		return file, ok, err
	}

	if fallbackConfigArg := sparkFallbackConfigArg(envVars); fallbackConfigArg != "" {
		configPath, resolved := resolveConfigPath(fallbackConfigArg, runtimeWorkingDir)
		if !resolved {
			return configfilesdiscoveryimpl.ConfigFile{}, false, nil
		}
		file, err = reader.ReadFile(ctx, configPath)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return configfilesdiscoveryimpl.ConfigFile{}, false, ctxErr
			}
			return configfilesdiscoveryimpl.ConfigFile{}, false, nil
		}
		return file, true, nil
	}

	if !sparkHasTaggedSubmitDriver(ctx, reader) {
		return configfilesdiscoveryimpl.ConfigFile{}, false, nil
	}

	return readConfigFile(ctx, reader, sparkGetPropertiesFileFromCommandline, sparkCommandlineDoesNotBlockDefaultPaths, "", sparkDefaultConfigPathGroups...)
}

func readSparkExplicitPropertiesFile(
	ctx context.Context,
	reader configfilesdiscoveryimpl.ConfigReader,
) (configfilesdiscoveryimpl.ConfigFile, bool, bool, string, error) {
	runtimeWorkingDir := ""
	configPath := ""
	explicitFound := false

	if commandline, err := reader.ReadRuntimeCommandline(ctx); err == nil {
		runtimeWorkingDir = commandline.WorkingDir
		if configArg, found := sparkGetPropertiesFileFromCommandline(commandline.Args); found {
			explicitFound = true
			if resolvedPath, resolved := resolveConfigPath(configArg, commandline.WorkingDir); resolved {
				configPath = resolvedPath
			}
		}
	}

	if configPath == "" {
		for _, commandline := range reader.ReadLiveProcessCommandlines(ctx) {
			configArg, found := sparkGetPropertiesFileFromCommandline(commandline.Args)
			if !found {
				continue
			}
			explicitFound = true
			resolvedPath, resolved := resolveConfigPath(configArg, commandline.WorkingDir)
			if !resolved {
				return configfilesdiscoveryimpl.ConfigFile{}, false, true, runtimeWorkingDir, nil
			}
			if configPath != "" && configPath != resolvedPath {
				return configfilesdiscoveryimpl.ConfigFile{}, false, true, runtimeWorkingDir, nil
			}
			configPath = resolvedPath
		}
	}

	if configPath == "" {
		return configfilesdiscoveryimpl.ConfigFile{}, false, explicitFound, runtimeWorkingDir, nil
	}

	file, err := reader.ReadFile(ctx, configPath)
	if err != nil {
		return configfilesdiscoveryimpl.ConfigFile{}, false, true, runtimeWorkingDir, err
	}
	return file, true, true, runtimeWorkingDir, nil
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
			if !strings.HasPrefix(args[i], "-") {
				return "", false
			}
			if sparkSubmitOptionsWithValue[args[i]] && i+1 < len(args) {
				i++
			}
		}
		return "", false
	}
	return "", false
}

var sparkSubmitOptionsWithValue = map[string]bool{
	"--archives":             true,
	"--class":                true,
	"--conf":                 true,
	"--deploy-mode":          true,
	"--driver-class-path":    true,
	"--driver-cores":         true,
	"--driver-java-options":  true,
	"--driver-library-path":  true,
	"--driver-memory":        true,
	"--driver-resource":      true,
	"--exclude-packages":     true,
	"--executor-cores":       true,
	"--executor-memory":      true,
	"--executor-resource":    true,
	"--files":                true,
	"--jars":                 true,
	"--keytab":               true,
	"--kill":                 true,
	"--master":               true,
	"--name":                 true,
	"--num-executors":        true,
	"--packages":             true,
	"--principal":            true,
	"--proxy-user":           true,
	"--py-files":             true,
	"--queue":                true,
	"--remote":               true,
	"--repositories":         true,
	"--resource":             true,
	"--status":               true,
	"--total-executor-cores": true,
}

func sparkMatchesSubmitCommandline(args []string) bool {
	for _, arg := range unwrapShellCommandline(args) {
		if arg == sparkSubmitClass || path.Base(arg) == "spark-submit" {
			return true
		}
	}
	return false
}

// sparkCommandlineDoesNotBlockDefaultPaths lets SparkSubmit drivers without an
// explicit --properties-file continue to the standard spark-defaults.conf
// locations. Explicit properties-file arguments are still discovered by
// sparkGetPropertiesFileFromCommandline and remain authoritative.
func sparkCommandlineDoesNotBlockDefaultPaths([]string) bool {
	return false
}

func sparkHasTaggedSubmitDriver(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) bool {
	if commandline, err := reader.ReadRuntimeCommandline(ctx); err == nil && sparkSubmitHasDriverRoleTag(unwrapShellCommandline(commandline.Args)) {
		return true
	}
	for _, commandline := range reader.ReadLiveProcessCommandlines(ctx) {
		if sparkSubmitHasDriverRoleTag(unwrapShellCommandline(commandline.Args)) {
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
	args = unwrapShellCommandline(args)
	for _, arg := range args {
		if arg == sparkStandaloneDriverClass {
			return true
		}
	}

	return sparkSubmitHasDriverRoleTag(args)
}

func sparkSubmitHasDriverRoleTag(args []string) bool {
	for i, arg := range args {
		if arg != sparkSubmitClass {
			continue
		}
		for _, tagArg := range args[:i] {
			if strings.HasPrefix(tagArg, sparkDDTagsPrefix) && sparkTagsContainDriverRole(strings.TrimPrefix(tagArg, sparkDDTagsPrefix)) {
				return true
			}
		}
	}
	return false
}

func sparkTagsContainDriverRole(tags string) bool {
	for _, tag := range strings.Split(tags, ",") {
		if strings.TrimSpace(tag) == sparkDriverRoleTag {
			return true
		}
	}
	return false
}

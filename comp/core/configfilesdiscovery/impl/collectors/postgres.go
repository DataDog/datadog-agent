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
	PostgresIntegrationName     = "postgres"
	postgresConfigPayloadFormat = agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_PROPERTIES
	postgresConfigFileName      = "postgresql.conf"
)

type postgresConfigCollector struct{}

// postgresEnvAllow lists the only environment variables this collector ever
// forwards. PostgreSQL's official image and PGDATA/PG* variables otherwise
// carry credentials, connection strings, or arbitrary argument bags.
var postgresEnvAllow = map[string]struct{}{
	"PGDATA":                    {},
	"POSTGRES_DB":               {},
	"POSTGRES_USER":             {},
	"POSTGRES_HOST_AUTH_METHOD": {},
	"POSTGRES_INITDB_WALDIR":    {},
}

// postgresEnvDeny is defense in depth and documentation: the allow-list above
// already excludes every name here. Keeping the exclusion explicit protects
// against the allow-list being loosened later without revisiting these.
var postgresEnvDeny = map[string]struct{}{
	"POSTGRES_PASSWORD":      {},
	"POSTGRES_PASSWORD_FILE": {},
	"POSTGRES_INITDB_ARGS":   {},
	"PGPASSWORD":             {},
	"PGPASSFILE":             {},
	"PGSERVICE":              {},
	"PGSERVICEFILE":          {},
	"PGOPTIONS":              {},
	"PGSSLKEY":               {},
	"PGSSLCERT":              {},
	"PGSSLROOTCERT":          {},
	"PGREQUIRESSL":           {},
	"DATABASE_URL":           {},
	"POSTGRES_URL":           {},
}

func NewPostgres() configfilesdiscoveryimpl.ConfigCollector {
	return postgresConfigCollector{}
}

// CanCollectFromProcess returns whether the command line contains an explicit,
// resolvable PostgreSQL config source.
func (postgresConfigCollector) CanCollectFromProcess(commandline configfilesdiscoveryimpl.TargetCommandline) bool {
	configArg, ok := postgresGetConfigArgFromCommandline(commandline.Args)
	if !ok {
		return false
	}
	_, resolved := resolveConfigPath(configArg, commandline.WorkingDir)
	return resolved
}

func (c postgresConfigCollector) Collect(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) (configfilesdiscoveryimpl.CollectedConfig, error) {
	envVars, envErr := readEnvVars(ctx, reader, includePostgresEnvVar)
	if envErr != nil {
		log.Debugf("config files discovery skipped postgres env var collection: %v", envErr)
		envVars = nil
	}

	fallbackConfigArg := ""
	for _, envVar := range envVars {
		if envVar.Name == "PGDATA" && path.IsAbs(envVar.Value) {
			fallbackConfigArg = path.Join(envVar.Value, postgresConfigFileName)
			break
		}
	}

	file, ok, err := readConfigFile(
		ctx,
		reader,
		postgresGetConfigArgFromCommandline,
		postgresMatchesCommandline,
		fallbackConfigArg,
	)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("collect postgres config file: %w", err)
	}
	if !ok {
		// Without a config file, env vars are the only PostgreSQL config
		// source. Return the error so the scheduler retries.
		if envErr != nil {
			return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read postgres env vars: %w", envErr)
		}
		if len(envVars) == 0 {
			log.Debugf("config files discovery skipped postgres config collection: no config file or selected env vars detected")
			return configfilesdiscoveryimpl.CollectedConfig{}, nil
		}

		log.Debugf("config files discovery collected postgres env vars without a config file")
		return configfilesdiscoveryimpl.CollectedConfig{
			EnvVars: envVars,
		}, nil
	}

	file.PayloadFormat = postgresConfigPayloadFormat

	return configfilesdiscoveryimpl.CollectedConfig{
		ConfigFiles: []configfilesdiscoveryimpl.ConfigFile{file},
		EnvVars:     envVars,
	}, nil
}

func includePostgresEnvVar(name string) bool {
	if configfilesdiscoveryimpl.IsSecretEnvVarName(name) {
		return false
	}
	if _, denied := postgresEnvDeny[name]; denied {
		return false
	}
	_, allowed := postgresEnvAllow[name]
	return allowed
}

// postgresGetConfigArgFromCommandline returns the explicit PostgreSQL config
// source found in argv, in resolvable-path form: an explicit config_file
// setting, or the postgresql.conf implied by an explicit data directory.
func postgresGetConfigArgFromCommandline(args []string) (string, bool) {
	args = unwrapShellCommandline(args)
	postgresArgs, ok := postgresGetArgs(args)
	if !ok {
		return "", false
	}
	return postgresGetConfigArg(postgresArgs)
}

func postgresMatchesCommandline(args []string) bool {
	_, ok := postgresGetArgs(unwrapShellCommandline(args))
	return ok
}

func postgresGetArgs(args []string) ([]string, bool) {
	for i, arg := range args {
		if path.Base(arg) == "postgres" {
			return args[i+1:], true
		}
	}
	return nil, false
}

// postgresGetConfigArg scans postgres argv for an explicit config_file
// run-time parameter (`-c config_file=...` or `--config_file=...`) or an
// explicit data directory (`-D ...`), the last occurrence of either winning.
// An explicit config_file always takes precedence over a data directory, and
// an empty or otherwise unusable explicit value is still reported as found so
// the shared helper blocks the PGDATA fallback and default paths rather than
// guessing.
func postgresGetConfigArg(postgresArgs []string) (string, bool) {
	configFileFound := false
	configFileArg := ""
	dataDirFound := false
	dataDirArg := ""

	for i := 0; i < len(postgresArgs); i++ {
		arg := postgresArgs[i]

		if value, ok := strings.CutPrefix(arg, "--config_file="); ok {
			configFileFound = true
			configFileArg = value
			continue
		}
		if arg == "-c" && i+1 < len(postgresArgs) {
			if value, ok := strings.CutPrefix(postgresArgs[i+1], "config_file="); ok {
				configFileFound = true
				configFileArg = value
				i++
				continue
			}
		}
		if arg == "-D" && i+1 < len(postgresArgs) {
			dataDirFound = true
			dataDirArg = postgresArgs[i+1]
			i++
			continue
		}
		if value, ok := strings.CutPrefix(arg, "-D"); ok && value != "" {
			dataDirFound = true
			dataDirArg = value
			continue
		}
	}

	if configFileFound {
		return configFileArg, true
	}
	if dataDirFound {
		if dataDirArg == "" {
			return "", true
		}
		return path.Join(dataDirArg, postgresConfigFileName), true
	}
	return "", false
}

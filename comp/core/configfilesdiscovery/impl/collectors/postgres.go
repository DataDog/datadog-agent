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

// PostgresIntegrationName is the Autodiscovery check name for PostgreSQL.
const (
	PostgresIntegrationName     = "postgres"
	postgresConfigPayloadFormat = agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_PROPERTIES
)

type postgresConfigCollector struct{}

// postgresEnvAllow lists the documented PostgreSQL server and official-image
// settings whose values are configuration metadata. Bitnami's POSTGRESQL_*
// settings are handled by postgresBitnamiEnvAllow below.
//
// Keep POSTGRES_* explicit: that namespace contains passwords, credentials,
// and free-form argument values.
var postgresEnvAllow = map[string]struct{}{
	// PostgreSQL server environment variables.
	"PGCLIENTENCODING":    {},
	"PGDATA":              {},
	"PGDATESTYLE":         {},
	"PGPORT":              {},
	"PG_OOM_ADJUST_FILE":  {},
	"PG_OOM_ADJUST_VALUE": {},

	// Official PostgreSQL image variables.
	"POSTGRES_DB":            {},
	"POSTGRES_INITDB_WALDIR": {},
	"POSTGRES_USER":          {},

	// Bitnami POSTGRES_* aliases for the settings above.
	"POSTGRES_CLUSTER_APP_NAME":         {},
	"POSTGRES_INITDB_WAL_DIR":           {},
	"POSTGRES_MASTER_HOST":              {},
	"POSTGRES_MASTER_PORT_NUMBER":       {},
	"POSTGRES_NUM_SYNCHRONOUS_REPLICAS": {},
	"POSTGRES_PORT_NUMBER":              {},
	"POSTGRES_REPLICATION_MODE":         {},
	"POSTGRES_REPLICATION_USER":         {},
	"POSTGRES_SHUTDOWN_MODE":            {},
	"POSTGRES_SYNCHRONOUS_COMMIT_MODE":  {},
}

// postgresBitnamiEnvAllow accepts Bitnami's configuration namespace. The
// shared secret-name policy and postgresBitnamiEnvDeny reject unsafe settings
// first.
var postgresBitnamiEnvAllow = regexp.MustCompile(`^POSTGRESQL_[A-Z0-9_]+$`)

// postgresBitnamiEnvDeny rejects settings whose values can contain a secret
// or an unbounded argument string. Paths are configuration metadata; the
// collector does not read their referenced files.
var postgresBitnamiEnvDeny = regexp.MustCompile(
	`^POSTGRESQL_(?:.*_)?(?:ARGS|AUTH_METHOD|CONNECTION_STRING|DSN|FLAGS|OPTS|URI|URL)$`,
)

// NewPostgres returns a collector that reads selected, non-secret PostgreSQL
// environment variables and the main PostgreSQL configuration file.
func NewPostgres() configfilesdiscoveryimpl.ConfigCollector {
	return postgresConfigCollector{}
}

// CanCollectFromProcess returns whether the process command line identifies a
// PostgreSQL server. Its main config file is selected by an explicit command
// line option or by PGDATA.
func (postgresConfigCollector) CanCollectFromProcess(commandline configfilesdiscoveryimpl.TargetCommandline) bool {
	return postgresMatchesCommandline(commandline.Args)
}

func (postgresConfigCollector) Collect(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) (configfilesdiscoveryimpl.CollectedConfig, error) {
	envVars, envErr := readEnvVars(ctx, reader, includePostgresEnvVar)
	if envErr != nil {
		log.Debugf("config files discovery skipped postgres env var collection: %v", envErr)
		envVars = nil
	}

	fallbackConfigArg := postgresFallbackConfigArg(envVars)
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
		// Without a config file, env vars are the only PostgreSQL config source.
		// Return the error so the scheduler retries.
		if envErr != nil {
			return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read postgres env vars: %w", envErr)
		}
		if len(envVars) == 0 {
			log.Debugf("config files discovery skipped postgres config collection: no config file or selected env vars detected")
			return configfilesdiscoveryimpl.CollectedConfig{}, nil
		}

		log.Debugf("config files discovery collected postgres env vars without a config file")
		return configfilesdiscoveryimpl.CollectedConfig{EnvVars: envVars}, nil
	}

	file.PayloadFormat = postgresConfigPayloadFormat
	return configfilesdiscoveryimpl.CollectedConfig{
		ConfigFiles: []configfilesdiscoveryimpl.ConfigFile{file},
		EnvVars:     envVars,
	}, nil
}

// postgresFallbackConfigArg returns a config file path only for image
// conventions that expose one explicitly. PostgreSQL's PGDATA is a config
// directory, whereas Bitnami keeps its main configuration file separately.
func postgresFallbackConfigArg(envVars []configfilesdiscoveryimpl.ConfigEnvVar) string {
	var pgData string
	for _, envVar := range envVars {
		switch envVar.Name {
		case "POSTGRESQL_CONF_FILE":
			return envVar.Value
		case "PGDATA":
			pgData = envVar.Value
		}
	}
	if pgData == "" {
		return ""
	}
	return path.Join(pgData, "postgresql.conf")
}

// postgresGetConfigArgFromCommandline returns the main PostgreSQL config file
// specified by the server command. An explicit config_file takes precedence
// over -D because PostgreSQL permits the config file to live elsewhere.
func postgresGetConfigArgFromCommandline(args []string) (string, bool) {
	postgresArgs, ok := postgresGetArgs(unwrapShellCommandline(args))
	if !ok {
		return "", false
	}

	if configPath, found := postgresGetExplicitConfigArg(postgresArgs); found {
		return configPath, true
	}
	return postgresGetDataDirConfigArg(postgresArgs)
}

func postgresMatchesCommandline(args []string) bool {
	_, ok := postgresGetArgs(unwrapShellCommandline(args))
	return ok
}

func postgresGetArgs(args []string) ([]string, bool) {
	if len(args) == 0 {
		return nil, false
	}
	if path.Base(args[0]) == "postgres" {
		return args[1:], true
	}
	// Docker's official PostgreSQL image keeps docker-entrypoint.sh as the
	// container command and passes postgres as its first argument.
	if len(args) > 1 && path.Base(args[0]) == "docker-entrypoint.sh" && path.Base(args[1]) == "postgres" {
		return args[2:], true
	}
	return nil, false
}

func postgresGetExplicitConfigArg(args []string) (string, bool) {
	var configPath string
	foundConfigPath := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		var value string
		directPath := false
		switch {
		case arg == "-c" && i+1 < len(args):
			i++
			value = args[i]
		case strings.HasPrefix(arg, "-c"):
			value = strings.TrimPrefix(arg, "-c")
		case arg == "--config-file" && i+1 < len(args):
			i++
			value = args[i]
			directPath = true
		case strings.HasPrefix(arg, "--config-file="):
			value = strings.TrimPrefix(arg, "--config-file=")
			directPath = true
		case strings.HasPrefix(arg, "--config_file="):
			value = strings.TrimPrefix(arg, "--config_file=")
			directPath = true
		default:
			continue
		}

		if !directPath {
			name, path, found := strings.Cut(value, "=")
			if !found || (name != "config_file" && name != "config-file") {
				continue
			}
			value = path
		}

		configPath = value
		foundConfigPath = true
	}
	return configPath, foundConfigPath
}

func postgresGetDataDirConfigArg(args []string) (string, bool) {
	var dataDir string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		var value string
		switch {
		case arg == "-D" && i+1 < len(args):
			i++
			value = args[i]
		case strings.HasPrefix(arg, "-D"):
			value = strings.TrimPrefix(arg, "-D")
		default:
			continue
		}
		if value == "" {
			return "", true
		}
		dataDir = value
	}
	if dataDir == "" {
		return "", false
	}
	return path.Join(dataDir, "postgresql.conf"), true
}

func includePostgresEnvVar(name string) bool {
	if configfilesdiscoveryimpl.IsSecretEnvVarName(name) {
		return false
	}
	if postgresBitnamiEnvDeny.MatchString(name) {
		return false
	}
	_, allowed := postgresEnvAllow[name]
	return allowed || postgresBitnamiEnvAllow.MatchString(name)
}

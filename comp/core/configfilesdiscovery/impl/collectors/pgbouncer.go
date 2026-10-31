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

// PgbouncerIntegrationName is the Autodiscovery check name for PgBouncer.
const (
	PgbouncerIntegrationName     = "pgbouncer"
	pgbouncerConfigPayloadFormat = agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_INI
)

// pgbouncerDefaultConfigPaths are the conventional pgbouncer.ini locations of
// the images available for dogfooding: edoburu/pgbouncer and Bitnami. Both
// generate this file from a userlist.txt/auth_file kept separately, so unlike
// a hand-written pgbouncer.ini, it does not contain inline credentials.
var pgbouncerDefaultConfigPaths = []string{
	"/etc/pgbouncer/pgbouncer.ini",
	"/opt/bitnami/pgbouncer/conf/pgbouncer.ini",
}

type pgbouncerConfigCollector struct{}

// pgbouncerEnvAllow contains non-secret settings documented by the Bitnami
// wrapper (PGBOUNCER_*) and by edoburu/pgbouncer. The latter is intentionally
// a closed list: its unprefixed variables are generic, but it is the image
// currently available for dogfooding.
var pgbouncerEnvAllow = map[string]struct{}{
	// Bitnami.
	"PGBOUNCER_AUTH_HBA_FILE":       {},
	"PGBOUNCER_AUTH_IDENT_FILE":     {},
	"PGBOUNCER_AUTH_TYPE":           {},
	"PGBOUNCER_CONF_DIR":            {},
	"PGBOUNCER_CONF_FILE":           {},
	"PGBOUNCER_DEFAULT_POOL_SIZE":   {},
	"PGBOUNCER_LOG_DIR":             {},
	"PGBOUNCER_LOG_FILE":            {},
	"PGBOUNCER_MAX_CLIENT_CONN":     {},
	"PGBOUNCER_PID_FILE":            {},
	"PGBOUNCER_PORT":                {},
	"PGBOUNCER_SERVER_IDLE_TIMEOUT": {},
	"PGBOUNCER_STATS_USERS":         {},
	"PGBOUNCER_TMP_DIR":             {},

	// edoburu/pgbouncer dogfood image.
	"ADMIN_USERS":          {},
	"AUTH_TYPE":            {},
	"DB_HOST":              {},
	"DB_NAME":              {},
	"DB_PORT":              {},
	"DB_USER":              {},
	"DEFAULT_POOL_SIZE":    {},
	"LISTEN_ADDR":          {},
	"LISTEN_PORT":          {},
	"MAX_CLIENT_CONN":      {},
	"MAX_DB_CONNECTIONS":   {},
	"MAX_USER_CONNECTIONS": {},
	"MIN_POOL_SIZE":        {},
	"POOL_MODE":            {},
	"RESERVE_POOL_SIZE":    {},
	"SERVER_IDLE_TIMEOUT":  {},
	"SERVER_LIFETIME":      {},
	"STATS_USERS":          {},
}

// pgbouncerEnvDeny is defense in depth and documents values that must never
// be forwarded even if the allow-list is broadened later. The shared
// secret-name filter also rejects password-shaped names.
var pgbouncerEnvDeny = map[string]struct{}{
	"DATABASE_URL":          {},
	"DATABASE_URLS":         {},
	"DB_PASSWORD":           {},
	"PGBOUNCER_AUTH_QUERY":  {},
	"PGBOUNCER_EXTRA_FLAGS": {},
	"SERVER_RESET_QUERY":    {},
}

func NewPgbouncer() configfilesdiscoveryimpl.ConfigCollector {
	return pgbouncerConfigCollector{}
}

// CanCollectFromProcess returns whether a process event identifies PgBouncer
// and can trigger the one-shot recollection fallback for its container.
func (pgbouncerConfigCollector) CanCollectFromProcess(commandline configfilesdiscoveryimpl.TargetCommandline) bool {
	return pgbouncerMatchesCommandline(commandline.Args)
}

// Collect gathers selected, non-secret PgBouncer environment variables and
// the main pgbouncer.ini file. It never reads userlist.txt or any auth_file:
// unlike pgbouncer.ini, those files hold credentials, including in plaintext
// for some auth_types.
func (pgbouncerConfigCollector) Collect(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) (configfilesdiscoveryimpl.CollectedConfig, error) {
	envVars, envErr := readEnvVars(ctx, reader, includePgbouncerEnvVar)
	if envErr != nil {
		log.Debugf("config files discovery skipped pgbouncer env var collection: %v", envErr)
		envVars = nil
	}

	fallbackConfigArg := pgbouncerFallbackConfigArg(envVars)
	selection, err := selectConfigFile(
		ctx,
		reader,
		pgbouncerGetConfigArgFromCommandline,
		pgbouncerMatchesCommandline,
		fallbackConfigArg,
		pgbouncerDefaultConfigPaths,
	)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("collect pgbouncer config file: %w", err)
	}
	if selection == nil {
		// Without a config file, env vars are the only PgBouncer config source.
		// Return the error so the scheduler retries.
		if envErr != nil {
			return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read pgbouncer env vars: %w", envErr)
		}
		if len(envVars) == 0 {
			log.Debugf("config files discovery skipped pgbouncer config collection: no config file or selected env vars detected")
			return configfilesdiscoveryimpl.CollectedConfig{}, nil
		}

		log.Debugf("config files discovery collected pgbouncer env vars without a config file")
		return configfilesdiscoveryimpl.CollectedConfig{EnvVars: envVars}, nil
	}

	file := selection.file
	file.PayloadFormat = pgbouncerConfigPayloadFormat
	return configfilesdiscoveryimpl.CollectedConfig{
		ConfigFiles: []configfilesdiscoveryimpl.ConfigFile{file},
		EnvVars:     envVars,
	}, nil
}

// pgbouncerFallbackConfigArg returns a config file path only for the Bitnami
// image, which exposes its config location through PGBOUNCER_CONF_FILE or
// PGBOUNCER_CONF_DIR. edoburu/pgbouncer exposes no such variable.
func pgbouncerFallbackConfigArg(envVars []configfilesdiscoveryimpl.ConfigEnvVar) string {
	var confDir string
	for _, envVar := range envVars {
		switch envVar.Name {
		case "PGBOUNCER_CONF_FILE":
			return envVar.Value
		case "PGBOUNCER_CONF_DIR":
			confDir = envVar.Value
		}
	}
	if confDir == "" {
		return ""
	}
	return path.Join(confDir, "pgbouncer.ini")
}

// pgbouncerGetConfigArgFromCommandline returns the CONFIG_FILE argument from
// PgBouncer's "pgbouncer [OPTION]... CONFIG_FILE" usage.
func pgbouncerGetConfigArgFromCommandline(args []string) (string, bool) {
	pgbouncerArgs, ok := pgbouncerGetArgs(unwrapShellCommandline(args))
	if !ok {
		return "", false
	}
	return pgbouncerGetConfigArg(pgbouncerArgs)
}

func pgbouncerMatchesCommandline(args []string) bool {
	_, ok := pgbouncerGetArgs(unwrapShellCommandline(args))
	return ok
}

func pgbouncerGetArgs(args []string) ([]string, bool) {
	if len(args) == 0 {
		return nil, false
	}
	if path.Base(args[0]) == "pgbouncer" {
		return args[1:], true
	}
	return nil, false
}

// pgbouncerGetConfigArg returns the single positional CONFIG_FILE argument.
// Every other documented option is a boolean flag except -u/--user, which
// takes a separate value. An unrecognized option is left unhandled rather
// than guessed at.
func pgbouncerGetConfigArg(args []string) (string, bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "":
			return "", false
		case arg == "-u" || arg == "--user":
			i++
		case strings.HasPrefix(arg, "--user="):
		case pgbouncerIsBooleanFlag(arg):
		case strings.HasPrefix(arg, "-"):
			return "", false
		default:
			return arg, true
		}
	}
	return "", false
}

func pgbouncerIsBooleanFlag(arg string) bool {
	switch arg {
	case "-d", "--daemon", "-q", "--quiet", "-R", "--reboot", "-v", "--verbose", "-V", "--version", "-h", "--help":
		return true
	default:
		return false
	}
}

func includePgbouncerEnvVar(name string) bool {
	if configfilesdiscoveryimpl.IsSecretEnvVarName(name) {
		return false
	}
	if _, denied := pgbouncerEnvDeny[name]; denied {
		return false
	}
	_, allowed := pgbouncerEnvAllow[name]
	return allowed
}

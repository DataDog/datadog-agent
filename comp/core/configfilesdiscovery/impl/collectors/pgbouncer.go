// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"fmt"

	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PgbouncerIntegrationName is the Autodiscovery check name for PgBouncer.
const PgbouncerIntegrationName = "pgbouncer"

type pgbouncerConfigCollector struct{}

// pgbouncerEnvAllow contains non-secret settings documented by the Bitnami
// wrapper (PGBOUNCER_*) and by edoburu/pgbouncer. The latter is intentionally
// a closed list: its unprefixed variables are generic, but it is the image
// currently available for dogfooding. As with the PostgreSQL env-var
// collector, Autodiscovery selects the target container from its PgBouncer
// check configuration; this collector does not inspect process command lines.
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

func NewPgbouncer() configfilesdiscoveryimpl.ConfigCollector {
	return pgbouncerConfigCollector{}
}

func (pgbouncerConfigCollector) CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline) bool {
	return false
}

// Collect intentionally gathers environment metadata only. Reading
// pgbouncer.ini is deferred to a separate change because its [databases]
// section can contain inline credentials.
func (pgbouncerConfigCollector) Collect(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) (configfilesdiscoveryimpl.CollectedConfig, error) {
	envVars, err := readEnvVars(ctx, reader, includePgbouncerEnvVar)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read pgbouncer env vars: %w", err)
	}
	if len(envVars) == 0 {
		log.Debugf("config files discovery skipped pgbouncer env var collection: no selected env vars detected")
		return configfilesdiscoveryimpl.CollectedConfig{}, nil
	}
	return configfilesdiscoveryimpl.CollectedConfig{EnvVars: envVars}, nil
}

func includePgbouncerEnvVar(name string) bool {
	if configfilesdiscoveryimpl.IsSecretEnvVarName(name) {
		return false
	}
	_, allowed := pgbouncerEnvAllow[name]
	return allowed
}
